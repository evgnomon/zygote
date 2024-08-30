package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/internal/mem"
	"github.com/evgnomon/zygote/internal/migration"
	"github.com/evgnomon/zygote/internal/util"
)

const containerStartTimeout = 20 * time.Second

func main() {
	logger, err := util.Logger()
	if err != nil {
		panic(err)
	}

	app := &cli.App{
		Commands: []*cli.Command{
			{
				Name:  "migrate",
				Usage: "Manage database migrations. Allows you to apply or revert changes to the database schema.",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "directory",
						Aliases: []string{"C"},
						Value:   "sqls",
						Usage:   "Directory containing the SQL migration files.",
					},
				},
				Subcommands: []*cli.Command{
					{
						Name:  "up",
						Usage: "Apply all pending migrations to update the database schema to the latest version.",
						Action: func(c *cli.Context) error {
							ctx := context.Background()
							m := NewMigration(c.String("directory"))
							return m.Up(ctx, logger)
						},
					},
					{
						Name:  "down",
						Usage: "Revert the last applied migration to undo changes to the database schema.",
						Action: func(c *cli.Context) error {
							ctx := context.Background()
							m := NewMigration(c.String("directory"))
							return m.Down(ctx, logger)
						},
					},
				},
			},
			{
				Name:  "init",
				Usage: "Initialize DB and Mem containers for a new project",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "directory",
						Aliases: []string{"C"},
						Value:   "sqls",
						Usage:   "Directory containing the SQL migration files.",
					},
				},
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					err := initContainers(ctx, logger, c.String("directory"))
					return err
				},
			},
			{
				Name:  "deinit",
				Usage: "Clean up DB and Mem containers created by zygote",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "remove-volume",
						Aliases: []string{"v"},
						Usage:   "Remove volumes associated with the containers",
					},
				},
				Action: func(c *cli.Context) error {
					for _, v := range container.List() {
						if strings.HasPrefix(v.Name, "/zygote-") {
							container.RemoveContainer(v.ID)
						}
					}
					if c.Bool("remove-volume") {
						container.RemoveVolumePrefix("zygote-")
					}
					return nil
				},
			},
			{
				Name:  "run",
				Usage: "Run the application",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "init",
						Usage: "Remove volumes associated with the containers",
					},
					&cli.StringFlag{
						Name:    "directory",
						Aliases: []string{"C"},
						Value:   "sqls",
						Usage:   "Directory containing the SQL migration files.",
					},
				},
				Action: func(c *cli.Context) error {
					for _, v := range container.List() {
						if strings.HasPrefix(v.Name, "/zygote-") {
							container.RemoveContainer(v.ID)
						}
					}
					if c.Bool("init") {
						container.RemoveVolumePrefix("zygote-")
					}
					err := initContainers(context.Background(), logger, c.String("directory"))
					return err
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		logger.Error("error running command", zap.Error(err))
	}
}

func NewMigration(directory string) *migration.Migration {
	return &migration.Migration{
		Directory: directory,
	}
}

func initContainers(ctx context.Context, logger *zap.Logger, directory string) error {
	numShards := 2
	for i := 1; i <= numShards; i++ {
		sqlParams := container.SQLInitParams{
			DBName:   fmt.Sprintf("myproject_%d", i),
			Username: fmt.Sprintf("test_%d", i),
			Password: "password",
		}
		sqlStatements, err := container.SQLInit(sqlParams)
		if err != nil {
			return err
		}

		container.Vol(sqlStatements, fmt.Sprintf("zygote-db-conf-%d", i), "/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())
	}
	db.CreateDBContainer(2, container.AppNetworkName())
	mem.CreateMemContainer(3, container.AppNetworkName())
	container.InitRedisCluster()
	container.WaitHealthy("zygote-", containerStartTimeout)
	m := NewMigration(directory)
	return m.Up(ctx, logger)
}
