// Package commands contains all available commands.
package commands

import (
	"fmt"

	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

const varChar255 = "VARCHAR(255)"

func sqlCol(colType string) string {
	var sqlCol string
	switch colType {
	case "string":
		sqlCol = varChar255
	case "integer":
		sqlCol = "INT"
	case "double":
		sqlCol = "DOUBLE"
	case "bool":
		sqlCol = "BOOLEAN"
	case "binary":
		sqlCol = "MEDIUMBLOB"
	case "json":
		sqlCol = "JSON"
	case "uuid":
		sqlCol = "CHAR(36)"
	case "text":
		sqlCol = "MEDIUMTEXT"
	default:
		sqlCol = varChar255
	}
	return sqlCol
}

func generateDBCommand() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "Create a database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			dbName := c.String("name")
			if dbName == "" {
				name := utils.RepoFullName()
				dbName = name
			}
			m, err := db.CreateDatabase(dbName)
			if err != nil {
				return fmt.Errorf("failed to create database: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateTableCommand() *cli.Command {
	return &cli.Command{
		Name:    "table",
		Usage:   "Create a table",
		Aliases: []string{"tab"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			tableName := c.String("name")
			if tableName == "" {
				return fmt.Errorf("name is required")
			}
			dbName := c.String("db")
			if dbName == "" {
				name := utils.RepoFullName()
				dbName = name
			}

			m, err := db.CreateTable(dbName, tableName)
			if err != nil {
				return fmt.Errorf("failed to create table: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateColCommand() *cli.Command {
	return &cli.Command{
		Name:    "column",
		Aliases: []string{"col"},
		Usage:   "Create a column",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the column",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.StringFlag{
				Name:     "table",
				Usage:    "Name of the table",
				Aliases:  []string{"t"},
				Required: true,
			},
			&cli.StringFlag{
				Name:  "type",
				Usage: "Column type: string, integer, double, bool, binary, json, text, uuid",
				Value: "string",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name := utils.RepoFullName()
				dbName = name
			}

			colType := c.String("type")
			if colType == "" {
				colType = "string"
			}

			m, err := db.CreateColumn(dbName, tableName, colName, sqlCol(colType))
			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generatePropCommand() *cli.Command {
	return &cli.Command{
		Name:    "property",
		Aliases: []string{"prop"},
		Usage:   "Extract a field out of a JSON and store it in a new column",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the column",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "path",
				Usage:    "Field path in the JSON",
				Value:    "string",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "table",
				Aliases:  []string{"t"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.StringFlag{
				Name:  "type",
				Usage: "Column type: string, integer, double, bool, binary, json, text, uuid",
				Value: "string",
			},
			&cli.BoolFlag{
				Name:  "virtual",
				Usage: "Name of the virtual property",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			fieldPath := c.String("path")
			if fieldPath == "" {
				return fmt.Errorf("JSON field path is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name := utils.RepoFullName()
				dbName = name
			}

			m, err := db.CreateProperty(dbName, tableName, colName, fieldPath,
				sqlCol(c.String("type")), c.Bool("virtual"))
			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateIndexCommand() *cli.Command {
	return &cli.Command{
		Name:  "index",
		Usage: "Create an index",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the index",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "table",
				Aliases:  []string{"t"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:     "column",
				Aliases:  []string{"col"},
				Usage:    "Name of the column. Use more than once for multiple columns",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.BoolFlag{
				Name:    "unique",
				Usage:   "Create a unique index",
				Aliases: []string{"u"},
			},
			&cli.BoolFlag{
				Name:  "full-text",
				Usage: "Create a full text index",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name := utils.RepoFullName()
				dbName = name
			}

			if c.Bool("unique") && c.Bool("full-text") {
				return fmt.Errorf("cannot create both unique and full-text index")
			}

			m, err := db.GenCreateSQL(&db.CreateIndexParams{
				CreateSQLParams: db.CreateSQLParams{
					Type:         "index",
					DatabaseName: dbName,
					TableName:    tableName,
					Name:         colName,
				},
				Columns:  c.StringSlice("column"),
				Unique:   c.Bool("unique"),
				FullText: c.Bool("full-text"),
			})

			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

// GenerateCommand generates source files.
func GenerateCommand() *cli.Command {
	return &cli.Command{
		Name:    "generate",
		Usage:   "Generate source files",
		Aliases: []string{"gen"},
		Subcommands: []*cli.Command{
			generateDBCommand(),
			generateTableCommand(),
			generateColCommand(),
			generatePropCommand(),
			generateIndexCommand(),
		},
	}
}
