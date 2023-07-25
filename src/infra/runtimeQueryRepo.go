package infra

import (
	"errors"
	"log"
	"strings"

	"github.com/speedianet/sam/src/domain/entity"
	"github.com/speedianet/sam/src/domain/valueObject"
	infraHelper "github.com/speedianet/sam/src/infra/helper"
)

var PhpTimeZones = []string{
	"UTC",
	"Europe/London",
	"Europe/Paris",
	"Europe/Berlin",
	"America/New_York",
	"America/Los_Angeles",
	"America/Chicago",
	"Asia/Tokyo",
	"Asia/Shanghai",
	"Australia/Sydney",
	"Australia/Melbourne",
	"Africa/Johannesburg",
	"Pacific/Auckland",
	"America/Sao_Paulo",
	"Europe/Rome",
	"Europe/Madrid",
	"America/Toronto",
	"Asia/Dubai",
	"Asia/Kolkata",
	"Europe/Amsterdam",
	"Europe/Stockholm",
	"Europe/Istanbul",
	"America/Mexico_City",
	"America/Phoenix",
	"Asia/Hong_Kong",
	"Asia/Singapore",
	"America/Argentina/Buenos_Aires",
	"America/Denver",
	"America/Vancouver",
	"Europe/Vienna",
	"Europe/Zurich",
	"Europe/Prague",
	"Africa/Cairo",
	"Africa/Lagos",
	"Asia/Seoul",
	"Asia/Tehran",
	"Australia/Brisbane",
	"Australia/Perth",
	"America/Manaus",
	"America/Bogota",
	"America/Lima",
	"America/Montreal",
	"Asia/Jakarta",
	"Asia/Riyadh",
	"Europe/Warsaw",
	"Europe/Athens",
	"Europe/Helsinki",
	"Europe/Lisbon",
	"Africa/Nairobi",
	"Pacific/Honolulu",
}

type RuntimeQueryRepo struct {
}

func (r RuntimeQueryRepo) GetPhpVersionsInstalled() ([]valueObject.PhpVersion, error) {
	olsConfigFile := "/usr/local/lsws/conf/httpd_config.conf"
	output, err := infraHelper.RunCmd(
		"awk",
		"/extprocessor lsphp/{print $2}",
		olsConfigFile,
	)
	if err != nil {
		log.Printf("FailedToGetPhpVersions: %v", err)
		return nil, errors.New("FailedToGetPhpVersions")
	}

	phpVersions := []valueObject.PhpVersion{}
	for _, version := range strings.Split(output, "\n") {
		if version == "" {
			continue
		}

		version = strings.Replace(version, "lsphp", "", 1)
		phpVersion, err := valueObject.NewPhpVersion(version)
		if err != nil {
			continue
		}

		phpVersions = append(phpVersions, phpVersion)
	}

	return phpVersions, nil
}

func (r RuntimeQueryRepo) GetPhpVersion(
	hostname valueObject.Fqdn,
) (entity.PhpVersion, error) {
	vhconfFile := WsQueryRepo{}.GetVirtualHostConfFilePath(hostname)
	currentPhpVersionStr, err := infraHelper.RunCmd(
		"awk",
		"/lsapi:lsphp/ {gsub(/[^0-9]/, \"\", $2); print $2}",
		vhconfFile,
	)
	if err != nil {
		log.Printf("FailedToGetPhpVersion: %v", err)
		return entity.PhpVersion{}, errors.New("FailedToGetPhpVersion")
	}

	currentPhpVersion, err := valueObject.NewPhpVersion(currentPhpVersionStr)
	if err != nil {
		return entity.PhpVersion{}, errors.New("FailedToGetPhpVersion")
	}

	phpVersions, err := r.GetPhpVersionsInstalled()
	if err != nil {
		return entity.PhpVersion{}, errors.New("FailedToGetPhpVersion")
	}

	return entity.NewPhpVersion(currentPhpVersion, phpVersions), nil
}

func (r RuntimeQueryRepo) phpSettingFactory(
	setting string,
) (entity.PhpSetting, error) {
	if setting == "" {
		return entity.PhpSetting{}, errors.New("InvalidPhpSetting")
	}

	settingParts := strings.Split(setting, " ")
	if len(settingParts) != 2 {
		return entity.PhpSetting{}, errors.New("InvalidPhpSetting")
	}

	settingNameStr := settingParts[0]
	settingValueStr := settingParts[1]
	if settingNameStr == "" || settingValueStr == "" {
		return entity.PhpSetting{}, errors.New("InvalidPhpSetting")
	}

	settingName, err := valueObject.NewPhpSettingName(settingNameStr)
	if err != nil {
		return entity.PhpSetting{}, errors.New("InvalidPhpSettingName")
	}

	settingValue, err := valueObject.NewPhpSettingValue(settingValueStr)
	if err != nil {
		return entity.PhpSetting{}, errors.New("InvalidPhpSettingValue")
	}

	settingOptions := []valueObject.PhpSettingOption{}
	valuesToInject := []string{}

	switch settingValue.GetType() {
	case "bool":
		valuesToInject = []string{"On", "Off"}
	case "number":
		valuesToInject = []string{
			"30", "60", "120", "300", "600", "900", "1800", "3600", "7200",
		}
	case "byteSize":
		lastChar := settingValue[len(settingValue)-1]
		switch lastChar {
		case 'K':
			valuesToInject = []string{"4096K", "8192K", "16384K"}
		case 'M':
			valuesToInject = []string{"256M", "512M", "1024M", "2048M"}
		case 'G':
			valuesToInject = []string{"1G", "2G", "4G"}
		}
	}

	switch settingName {
	case "error_reporting":
		valuesToInject = []string{
			"E_ALL",
			"~E_ALL",
			"E_ALL & ~E_DEPRECATED & ~E_STRICT",
			"E_ALL & ~E_DEPRECATED & ~E_STRICT & ~E_NOTICE & ~E_WARNING",
			"E_ERROR|E_CORE_ERROR|E_COMPILE_ERROR",
		}
	case "date.timezone":
		valuesToInject = PhpTimeZones
	}

	if len(valuesToInject) > 0 {
		for _, valueToInject := range valuesToInject {
			settingOptions = append(
				settingOptions,
				valueObject.NewPhpSettingOptionPanic(valueToInject),
			)
		}
	}

	return entity.NewPhpSetting(settingName, settingValue, settingOptions), nil
}

func (r RuntimeQueryRepo) GetPhpSettings(
	hostname valueObject.Fqdn,
) ([]entity.PhpSetting, error) {
	vhconfFile := WsQueryRepo{}.GetVirtualHostConfFilePath(hostname)
	output, err := infraHelper.RunCmd(
		"sed",
		"-n",
		"/phpIniOverride\\s*{/,/}/ { /phpIniOverride\\s*{/d; /}/d; s/^[[:space:]]*//; s/[^[:space:]]*[[:space:]]//; p; }",
		vhconfFile,
	)
	if err != nil || output == "" {
		log.Printf("FailedToGetPhpSettings: %v", err)
		return nil, errors.New("FailedToGetPhpSettings")
	}

	phpSettings := []entity.PhpSetting{}
	for _, setting := range strings.Split(output, "\n") {
		phpSetting, err := r.phpSettingFactory(setting)
		if err != nil {
			continue
		}

		phpSettings = append(phpSettings, phpSetting)
	}

	return phpSettings, nil
}

func (r RuntimeQueryRepo) GetPhpConfigs(
	hostname valueObject.Fqdn,
) (entity.PhpConfigs, error) {
	phpVersion, err := r.GetPhpVersion(hostname)
	if err != nil {
		return entity.PhpConfigs{}, err
	}

	phpSettings, err := r.GetPhpSettings(hostname)
	if err != nil {
		return entity.PhpConfigs{}, err
	}

	phpModules, err := r.GetPhpModules(hostname)
	if err != nil {
		return entity.PhpConfigs{}, err
	}

	return entity.NewPhpConfigs(
		hostname,
		phpVersion,
		phpSettings,
		phpModules,
	), nil
}