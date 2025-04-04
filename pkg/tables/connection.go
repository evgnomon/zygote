package tables

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// ClientConfig holds database connection configuration
type ClientConfig struct {
	User     string
	Password string
	Host     string
	Port     int
	Database string
}

func NewClientConfig() *ClientConfig {
	c := &ClientConfig{
		User:     "root",
		Password: "root1234",
		Host:     "shard-a.zygote.run",
		Port:     6446,
		Database: "mysql",
	}

	return c
}

// DatabaseConnector defines the interface for database connection
type DatabaseConnector interface {
	Connect() (*sql.DB, error)
}

// Connect implements the DatabaseConnector interface for DBConfig
func (c ClientConfig) Connect() (*sql.DB, error) {
	// Construct DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
		c.User,
		c.Password,
		c.Host,
		c.Port,
		c.Database,
	)

	// Connect to database
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %v", err)
	}

	return db, nil
}
