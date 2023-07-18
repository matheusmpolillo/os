package databaseInfra

import (
	"errors"
	"log"
	"regexp"
	"strconv"
	"strings"

	"github.com/speedianet/sam/src/domain/entity"
	"github.com/speedianet/sam/src/domain/valueObject"
	infraHelper "github.com/speedianet/sam/src/infra/helper"
)

type MysqlDatabaseQueryRepo struct {
}

func mysqlCmd(cmd string) (string, error) {
	return infraHelper.RunCmd(
		"mysql",
		"--skip-column-names",
		"--silent",
		"--execute",
		cmd,
	)
}

func (repo MysqlDatabaseQueryRepo) getDatabaseNames() ([]valueObject.DatabaseName, error) {
	var dbNameList []valueObject.DatabaseName

	dbNameListStr, err := mysqlCmd("SHOW DATABASES")
	if err != nil {
		log.Printf("GetDatabaseNamesError: %v", err)
		return dbNameList, errors.New("GetDatabaseNamesError")
	}

	dbNameListSlice := strings.Split(dbNameListStr, "\n")
	dbExcludeRegex := "^(information_schema|mysql|performance_schema|sys)$"
	for _, dbName := range dbNameListSlice {
		if regexp.MustCompile(dbExcludeRegex).MatchString(dbName) {
			continue
		}
		dbName, err := valueObject.NewDatabaseName(dbName)
		if err != nil {
			continue
		}
		dbNameList = append(dbNameList, dbName)
	}

	return dbNameList, nil
}

func (repo MysqlDatabaseQueryRepo) getDatabaseSize(dbName valueObject.DatabaseName) (
	valueObject.Byte,
	error,
) {
	dbSizeStr, err := mysqlCmd(
		"SELECT SUM(data_length + index_length) FROM information_schema.TABLES WHERE table_schema = '" + dbName.String() + "'",
	)
	if err != nil {
		log.Printf("GetDatabaseSizeError: %v", err)
		return 0, errors.New("GetDatabaseSizeError")
	}

	dbSizeInBytes, err := strconv.ParseInt(dbSizeStr, 10, 64)
	if err != nil {
		return 0, err
	}

	return valueObject.Byte(dbSizeInBytes), nil
}

func (repo MysqlDatabaseQueryRepo) getDatabaseUsernames(
	dbName valueObject.DatabaseName,
) ([]valueObject.DatabaseUsername, error) {
	var dbUsernameList []valueObject.DatabaseUsername

	dbUserStr, err := mysqlCmd(
		"SELECT User FROM mysql.db WHERE Db = '" + dbName.String() + "'",
	)
	if err != nil {
		log.Printf("GetDatabaseUserError: %v", err)
		return dbUsernameList, errors.New("GetDatabaseUserError")
	}

	dbUserSlice := strings.Split(dbUserStr, "\n")
	for _, dbUser := range dbUserSlice {
		dbUser, err := valueObject.NewDatabaseUsername(dbUser)
		if err != nil {
			continue
		}
		dbUsernameList = append(dbUsernameList, dbUser)
	}

	return dbUsernameList, nil
}

func (repo MysqlDatabaseQueryRepo) getDatabaseUserPrivileges(
	dbName valueObject.DatabaseName,
	dbUser valueObject.DatabaseUsername,
) ([]valueObject.DatabasePrivilege, error) {
	var dbUserPrivileges []valueObject.DatabasePrivilege

	dbPrivilegesStr, err := mysqlCmd(
		"SHOW GRANTS FOR '" + dbUser.String(),
	)
	if err != nil {
		log.Printf("GetDatabaseUserPrivilegesError: %v", err)
		return dbUserPrivileges, errors.New("GetDatabaseUserPrivilegesError")
	}

	dbPrivilegesSlice := strings.Split(dbPrivilegesStr, "\n")
	for _, dbPrivilege := range dbPrivilegesSlice {
		dbPrivilege, err := valueObject.NewDatabasePrivilege(dbPrivilege)
		if err != nil {
			continue
		}
		dbUserPrivileges = append(dbUserPrivileges, dbPrivilege)
	}

	return dbUserPrivileges, nil
}

func (repo MysqlDatabaseQueryRepo) Get() ([]entity.Database, error) {
	dbNames, err := repo.getDatabaseNames()
	if err != nil {
		return []entity.Database{}, err
	}
	dbType, _ := valueObject.NewDatabaseType("mysql")

	var databases []entity.Database
	for _, dbName := range dbNames {
		dbSize, err := repo.getDatabaseSize(dbName)
		if err != nil {
			dbSize = valueObject.Byte(0)
		}

		dbUsernames, err := repo.getDatabaseUsernames(dbName)
		if err != nil {
			dbUsernames = []valueObject.DatabaseUsername{}
		}

		var dbUserPrivileges []valueObject.DatabasePrivilege
		for _, dbUser := range dbUsernames {
			dbUserPrivileges, err = repo.getDatabaseUserPrivileges(dbName, dbUser)
			if err != nil {
				dbUserPrivileges = []valueObject.DatabasePrivilege{}
			}
		}

		dbUsersWithPrivileges := []entity.DatabaseUser{}
		for _, dbUsername := range dbUsernames {
			dbUsersWithPrivileges = append(
				dbUsersWithPrivileges,
				entity.NewDatabaseUser(
					dbUsername,
					dbName,
					dbType,
					dbUserPrivileges,
				),
			)
		}

		databases = append(
			databases,
			entity.NewDatabase(
				dbName,
				dbType,
				dbSize,
				dbUsersWithPrivileges,
			),
		)
	}

	return databases, nil
}