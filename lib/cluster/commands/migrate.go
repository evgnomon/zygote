/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <https://evgnomon.org/docs/hgl>
*/
// Package commands contains all available commands.
package commands

import (
	"context"

	"github.com/evgnomon/zygote/lib/cluster/migration"
	"github.com/evgnomon/zygote/lib/cluster/utils"
	"github.com/urfave/cli/v2"
)

// MigrateCommand manages database migrations.
func MigrateCommand() *cli.Command {
	return &cli.Command{
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
		Action: func(c *cli.Context) error {
			dir := c.String("directory")
			if dir == "" {
				dir = "sqls"
			}
			sqlDirExists := utils.PathExists(dir)
			if !sqlDirExists {
				return nil
			}
			return nil
		},
		Subcommands: []*cli.Command{
			{
				Name:  "up",
				Usage: "Apply all pending migrations to update the database schema to the latest version.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := migration.NewMigration(c.String("directory"))
					return m.Up(ctx)
				},
			},
			{
				Name:  "down",
				Usage: "Revert the last applied migration to undo changes to the database schema.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := migration.NewMigration(c.String("directory"))
					return m.Down(ctx)
				},
			},
		},
	}
}
