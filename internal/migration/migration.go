package migration

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/tables"
	"github.com/evgnomon/zygote/pkg/utils"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
)

const routerReadWritePort = 6446
const routerReadOnlyPort = 6447
const defaultNumShards = 3

var logger = util.NewLogger()

type Migration struct {
	Directory string
	Connector *tables.MultiDBConnector
}

func (m *Migration) Up(_ context.Context) error {
	sqlDirExists := utils.PathExists(m.Directory)
	if !sqlDirExists {
		return nil
	}
	logger.Info("migrations up start", nil)
	db, err := m.Connector.GetWriteConnection(0)
	if err != nil {
		return err
	}
	m2, err := m.Migrate(db)
	if err != nil {
		return err
	}
	err = m2.Up()
	if err != nil {
		if err.Error() != "no change" {
			return err
		}
	}
	logger.Info("migrations up done", nil)
	return nil
}

func (m *Migration) Down(_ context.Context) error {
	sqlDirExists := utils.PathExists(m.Directory)
	if !sqlDirExists {
		return nil
	}
	logger.Info("migrations down start", nil)
	db, err := m.Connector.GetWriteConnection(0)
	if err != nil {
		return err
	}
	m2, err := m.Migrate(db)
	if err != nil {
		return err
	}
	err = m2.Down()
	if err != nil {
		if err.Error() != "no change" {
			return err
		}
	}
	_, _, err = m2.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return err
	}
	logger.Info("migrations down done", nil)
	return nil
}

// Migrate *sql.DB
func (m *Migration) Migrate(db *sql.DB) (*migrate.Migrate, error) {
	driver, err := mysql.WithInstance(db, &mysql.Config{})
	if err != nil {
		return nil, fmt.Errorf("dbMigrate WithInstance: %w", err)
	}
	mm, err := migrate.NewWithDatabaseInstance(
		fmt.Sprintf("file://%s", m.Directory),
		"mysql",
		driver,
	)
	if err != nil {
		return nil, fmt.Errorf("dbMigrate NewWithDatabaseInstance: %w", err)
	}
	return mm, nil
}

func NewMigration(directory string) *Migration {
	ctx := context.Background()
	connector := tables.NewMultiDBConnector(container.AppNetworkName(), "zygote", "my.zygote.run", "mysql",
		routerReadOnlyPort, routerReadWritePort, defaultNumShards)
	_, err := connector.ConnectAllShardsRead(ctx)
	logger.FatalIfErr("migration: connect all shards read", err)
	_, err = connector.ConnectAllShardsWrite(ctx)
	logger.FatalIfErr("migration: connect all shards write", err)
	return &Migration{
		Directory: directory,
		Connector: connector,
	}
}
