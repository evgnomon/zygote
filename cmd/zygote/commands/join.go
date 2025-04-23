// Package commands contains all available commands.
package commands

import (
	"context"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// JoinCommand joins to a remote cluster.
func JoinCommand() *cli.Command {
	return &cli.Command{
		Name:  "join",
		Usage: "Join to a remote cluster",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "directory",
				Aliases: []string{"C"},
				Value:   "sqls",
				Usage:   "Directory containing the SQL migration files.",
			},
			&cli.IntFlag{
				Name:  "num-shards",
				Value: 1,
				Usage: "number of shards",
			},
			&cli.IntFlag{
				Name:  "shard-size",
				Value: 3,
				Usage: "number of nodes in a shard",
			},
			&cli.StringFlag{
				Name:     "domain",
				Aliases:  []string{"d"},
				Usage:    "The domain name, e.g. foo.com or foo.bar.com",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "host",
				Usage:    "The host name, e.g. shard-a-1.foo.com or shard-b.foo.bar.com",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			ctx := context.Background()
			var sn = db.NewSQLNode()
			sn.Domain = c.String("domain")
			sn.NetworkName = container.HostNetworkName
			sn.DatabaseName = c.String("db")
			h, err := utils.CalculateIndices(c.String("host"))
			if err != nil {
				return err
			}
			sn.ShardIndex = h.ShardIndex
			sn.RepIndex = h.RepIndex
			sn.Tenant = utils.TenantName()
			sn.RootPassword = "root1234"
			sn.NumShards = c.Int("num-shards")
			sn.ShardSize = c.Int("shard-size")

			createNode(ctx, sn)
			return nil
		},
	}
}
