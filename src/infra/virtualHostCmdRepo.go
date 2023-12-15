package infra

import (
	"errors"

	"github.com/speedianet/os/src/domain/dto"
	"github.com/speedianet/os/src/domain/entity"
	"github.com/speedianet/os/src/domain/valueObject"
	infraHelper "github.com/speedianet/os/src/infra/helper"
)

type VirtualHostCmdRepo struct {
}

func (repo VirtualHostCmdRepo) reloadWebServer() error {
	_, err := infraHelper.RunCmd(
		"nginx",
		"-t",
	)
	if err != nil {
		return errors.New("NginxConfigTestFailed")
	}

	_, err = infraHelper.RunCmd(
		"nginx",
		"-s",
		"reload",
	)
	if err != nil {
		return errors.New("NginxReloadFailed")
	}

	return nil
}

func (repo VirtualHostCmdRepo) getAliasConfigFile(
	parentHostname valueObject.Fqdn,
) (valueObject.UnixFilePath, error) {
	vhostFileStr := "/app/conf/nginx/" + parentHostname.String() + ".conf"

	isParentPrimaryDomain := VirtualHostQueryRepo{}.IsVirtualHostPrimaryDomain(
		parentHostname,
	)
	if isParentPrimaryDomain {
		vhostFileStr = "/app/conf/nginx/primary.conf"
	}

	return valueObject.UnixFilePath(vhostFileStr), nil
}

func (repo VirtualHostCmdRepo) addAlias(addDto dto.AddVirtualHost) error {
	vhostFile, err := repo.getAliasConfigFile(*addDto.ParentHostname)
	if err != nil {
		return errors.New("GetAliasConfigFileFailed")
	}
	vhostFileStr := vhostFile.String()

	hostnameStr := addDto.Hostname.String()

	_, err = infraHelper.RunCmd(
		"sed",
		"-i",
		`/server_name/ s/;$/ `+hostnameStr+` www.`+hostnameStr+`;/`,
		vhostFileStr,
	)
	if err != nil {
		return errors.New("AddAliasFailed")
	}

	// TODO: Regenerate cert for primary domain to include new alias

	return repo.reloadWebServer()
}

func (repo VirtualHostCmdRepo) Add(addDto dto.AddVirtualHost) error {
	hostnameStr := addDto.Hostname.String()

	if addDto.Type.String() == "alias" {
		return repo.addAlias(addDto)
	}

	publicDir := "/app/html/" + hostnameStr
	certPath := "/app/conf/pki/" + hostnameStr + ".crt"
	keyPath := "/app/conf/pki/" + hostnameStr + ".key"
	mappingFilePath := "/app/conf/nginx/mapping/" + hostnameStr + ".conf"

	nginxConf := `server {
    listen 80;
    listen 443 ssl;
    server_name ` + hostnameStr + ` www.` + hostnameStr + `;

    root ` + publicDir + `;

    ssl_certificate ` + certPath + `;
    ssl_certificate_key ` + keyPath + `;

    access_log /app/logs/nginx/` + hostnameStr + `_access.log combined buffer=512k flush=1m;
    error_log /app/logs/nginx/` + hostnameStr + `_error.log warn;

    include /etc/nginx/std.conf;
    include ` + mappingFilePath + `;
}
`
	err := infraHelper.UpdateFile(
		"/app/conf/nginx/"+hostnameStr+".conf",
		nginxConf,
		true,
	)
	if err != nil {
		return errors.New("CreateNginxConfFileFailed")
	}

	err = infraHelper.UpdateFile(
		mappingFilePath,
		"",
		true,
	)
	if err != nil {
		return errors.New("CreateMappingFileFailed")
	}

	err = infraHelper.MakeDir(publicDir)
	if err != nil {
		return errors.New("MakePublicHtmlDirFailed")
	}

	_, err = infraHelper.RunCmd(
		"openssl",
		"req",
		"-x509",
		"-nodes",
		"-days",
		"365",
		"-newkey",
		"rsa:2048",
		"-keyout",
		keyPath,
		"-out",
		certPath,
		"-subj",
		"/C=US/ST=California/L=LosAngeles/O=Acme/CN="+hostnameStr,
	)
	if err != nil {
		return errors.New("GenerateSelfSignedCertFailed")
	}

	directories := []string{
		publicDir,
		"/app/conf/nginx",
		"/app/conf/pki",
	}
	for _, directory := range directories {
		_, err = infraHelper.RunCmd(
			"chown",
			"-R",
			"nobody:nogroup",
			directory,
		)
		if err != nil {
			return errors.New("ChownNecessaryDirectoriesFailed")
		}
	}

	return repo.reloadWebServer()
}

func (repo VirtualHostCmdRepo) deleteAlias(vhost entity.VirtualHost) error {
	vhostFile, err := repo.getAliasConfigFile(*vhost.ParentHostname)
	if err != nil {
		return errors.New("GetAliasConfigFileFailed")
	}
	vhostFileStr := vhostFile.String()

	hostnameStr := vhost.Hostname.String()

	_, err = infraHelper.RunCmd(
		"sed",
		"-i",
		`/server_name/ s/ `+hostnameStr+` www.`+hostnameStr+`//`,
		vhostFileStr,
	)
	if err != nil {
		return errors.New("DeleteAliasFailed")
	}

	return repo.reloadWebServer()
}

func (repo VirtualHostCmdRepo) Delete(vhost entity.VirtualHost) error {
	hostnameStr := vhost.Hostname.String()
	if vhost.Type.String() == "alias" {
		return repo.deleteAlias(vhost)
	}

	_, err := infraHelper.RunCmd(
		"rm",
		"-rf",
		"/app/html/"+hostnameStr,
		"/app/conf/nginx/"+hostnameStr+".conf",
		"/app/conf/pki/"+hostnameStr+".crt",
		"/app/conf/pki/"+hostnameStr+".key",
		"/app/conf/nginx/mapping/"+hostnameStr+".conf",
	)
	if err != nil {
		return errors.New("DeleteVirtualHostFailed")
	}

	return repo.reloadWebServer()
}

func (repo VirtualHostCmdRepo) getServiceUrl(
	name valueObject.ServiceName,
) (valueObject.Url, error) {
	var url valueObject.Url

	servicesList, err := ServicesQueryRepo{}.Get()
	if err != nil {
		return url, errors.New("GetServicesListFailed")
	}

	urlStr := ""
	for _, service := range servicesList {
		if service.Name != name {
			continue
		}
		// TODO: Add support for protocol and thus multiple ports
		urlStr = "http://localhost:" + service.Ports[0].String()
	}

	if urlStr == "" {
		return url, errors.New("ServiceNotFound")
	}

	return valueObject.NewUrl(urlStr)
}

func (repo VirtualHostCmdRepo) AddMapping(addMapping dto.AddMapping) error {
	matchPatternStr := addMapping.MatchPattern.String()
	modifier := ""
	switch matchPatternStr {
	case "contains", "ends-with":
		modifier = "~"
	case "equals":
		modifier = "="
	}

	pathStr := addMapping.Path.String()
	if matchPatternStr == "ends-with" {
		pathStr += "$"
	}

	locationUri := pathStr
	if modifier != "" {
		locationUri = modifier + " " + pathStr
	}

	url := ""
	if addMapping.TargetUrl != nil {
		url = addMapping.TargetUrl.String()
	}
	if addMapping.TargetType.String() == "service" {
		svcUrl, err := repo.getServiceUrl(*addMapping.TargetServiceName)
		if err != nil {
			return errors.New("GetServiceUrlFailed")
		}

		url = svcUrl.String()
	}

	responseCode := ""
	if addMapping.TargetHttpResponseCode != nil {
		responseCode = addMapping.TargetHttpResponseCode.String()
	}

	directiveWithValue := "proxy_pass " + url
	switch addMapping.TargetType.String() {
	case "response-code":
		directiveWithValue = "return " + responseCode
	case "url":
		directiveWithValue = "return " + responseCode + " " + url
	}

	mapping := `location ` + locationUri + ` {
    ` + directiveWithValue + `;
}
`

	vhostQueryRepo := VirtualHostQueryRepo{}
	mappingFilePath, err := vhostQueryRepo.GetVirtualHostMappingsFilePath(
		addMapping.Hostname,
	)
	if err != nil {
		return errors.New("GetVirtualHostMappingsFilePathFailed")
	}

	err = infraHelper.UpdateFile(
		mappingFilePath.String(),
		mapping,
		false,
	)
	if err != nil {
		return errors.New("AddMappingFailed")
	}

	return repo.reloadWebServer()
}