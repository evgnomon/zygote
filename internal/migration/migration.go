package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/utils"
	migrate "github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
)

type Migration struct {
	Directory string
}

func (m *Migration) Up(_ context.Context) error {
	logger := util.NewLogger()
	sqlDirExists, err := utils.PathExists(m.Directory)
	if err != nil {
		return fmt.Errorf("failed to check if directory exists: %w", err)
	}
	if !sqlDirExists {
		return nil
	}
	logger.Info("migrations up start", nil)
	db, err := connect(0)
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
	logger := util.NewLogger()
	sqlDirExists, err := utils.PathExists(m.Directory)
	if err != nil {
		return fmt.Errorf("failed to check if directory exists: %w", err)
	}
	if !sqlDirExists {
		return nil
	}
	logger.Info("migrations down start", nil)
	db, err := connect(0)
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
	m2.Close()

	db2, err := connect(0)
	if err != nil {
		return err
	}

	empty, err := isDatabaseEmpty(db2, 0)
	if err != nil {
		return err
	}
	if !empty {
		return fmt.Errorf("database is not empty")
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

// Check if database is empty with MySQL
func isDatabaseEmpty(db *sql.DB, _ int) (bool, error) {
	rows, err := db.Query("SHOW TABLES")
	if err != nil {
		return false, fmt.Errorf("isDatabaseEmpty: %w", err)
	}
	defer rows.Close()
	tables := make([]string, 0)
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return false, fmt.Errorf("isDatabaseEmpty: %w", err)
		}
		tables = append(tables, table)
	}
	// check the only table availbale is schema_migrations
	if len(tables) == 1 && tables[0] == "schema_migrations" {
		return true, nil
	}
	return false, nil
}

// make *sql.DB based on connection string
func connect(shard int) (*sql.DB, error) {
	shardPort := 16446 + shard
	shardHost := "localhost"
	if envShardHost := os.Getenv(fmt.Sprintf("DB_SHARD_%d_INTERNAL_HOST", shard+1)); envShardHost != "" {
		shardHost = envShardHost
	}
	if shardPortStr := os.Getenv(fmt.Sprintf("DB_SHARD_%d_INTERNAL_PORT", shard+1)); shardPortStr != "" {
		var err error
		shardPort, err = strconv.Atoi(shardPortStr)
		if err != nil {
			return nil, fmt.Errorf("dbMigrate strconv.Atoi: %w", err)
		}
	}
	dbName, err := utils.RepoFullName()
	if err != nil {
		return nil, fmt.Errorf("dbMigrate RepoFullName: %w", err)
	}
	db, err := sql.Open(
		"mysql",
		fmt.Sprintf("admin:password@tcp(%[2]s:%[1]d)/%[3]s?multiStatements=true",
			shardPort, shardHost, dbName),
	)
	if err != nil {
		return nil, fmt.Errorf("dbMigrate open: %w", err)
	}
	return db, nil
}
