package migration

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"go.uber.org/zap"
)

type Migration struct {
	Directory string
}

func (m *Migration) Up(_ context.Context, logger *zap.Logger) error {
	logger.Info("migrations up start")
	for shardID := 0; shardID < 2; shardID++ {
		db, err := connect(shardID)
		if err != nil {
			return err
		}
		m, err := m.Migrate(logger, db)
		if err != nil {
			return err
		}
		err = m.Up()
		if err != nil {
			if err.Error() == "no change" {
				continue
			}
			return err
		}
	}
	logger.Info("migrations up done")
	return nil
}

func (m *Migration) Down(_ context.Context, logger *zap.Logger) error {
	logger.Info("migrations down start")
	for shardID := 0; shardID < 2; shardID++ {
		db, err := connect(shardID)
		if err != nil {
			return err
		}
		m, err := m.Migrate(logger, db)
		if err != nil {
			return err
		}
		err = m.Down()
		if err != nil {
			if err.Error() == "no change" {
				continue
			}
			return err
		}

		_, _, err = m.Version()
		if err != nil && err != migrate.ErrNilVersion {
			return err
		}
		m.Close()

		db2, err := connect(shardID)
		if err != nil {
			return err
		}

		empty, err := isDatabaseEmpty(db2, shardID)
		if err != nil {
			return err
		}
		if !empty {
			return fmt.Errorf("database is not empty")
		}
	}
	logger.Info("migrations down done")
	return nil
}

// Migrate *sql.DB
func (m *Migration) Migrate(_ *zap.Logger, db *sql.DB) (*migrate.Migrate, error) {
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
	shardPort := 3306 + shard
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
	db, err := sql.Open(
		"mysql",
		fmt.Sprintf("test_%[1]d:password@tcp(%[3]s:%[2]d)/myproject_%[1]d?multiStatements=true",
			shard+1,
			shardPort,
			shardHost),
	)
	if err != nil {
		return nil, fmt.Errorf("dbMigrate open: %w", err)
	}
	return db, nil
}
