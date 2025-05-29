// Package commands contains all available commands.
package commands

import (
	"context"
	"sync"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/internal/mem"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

const defaultShardSize = 3
const defaultNumShards = 3

var logger = utils.NewLogger()

func createNode(ctx context.Context, sn *db.SQLNode) {
	n := container.NewNetworkConfig(sn.NetworkName)
	n.Ensure(ctx)
	err := sn.StartSQLContainers(ctx)
	logger.FatalIfErr("Make SQL node", err)
	mc := mem.NewMemNode()
	mc.Domain = sn.Domain
	mc.Tenant = sn.Tenant
	mc.ShardIndex = sn.ShardIndex
	mc.RepIndex = sn.RepIndex
	mc.NetworkName = sn.NetworkName
	if mc.NetworkName != container.HostNetworkName {
		mc.NetworkName = container.AppNetworkName()
	}
	mc.ShardSize = sn.ShardSize
	mc.NumShards = sn.NumShards
	mc.CreateReplica(ctx)
	mc.Init(ctx)
}

// InitCommand initializes resources for the current repo.
func InitCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize resources for the current repo",
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
			var wg sync.WaitGroup

			for shardIndex := 0; shardIndex < defaultNumShards; shardIndex++ {
				for repIndex := 0; repIndex < defaultShardSize; repIndex++ {
					sn := db.NewSQLNode()
					sn.Domain = utils.DomainName()
					sn.DatabaseName = utils.TenantName()
					sn.Tenant = utils.TenantName()
					sn.ShardIndex = shardIndex
					sn.RepIndex = repIndex
					sn.NumShards = defaultNumShards
					sn.ShardSize = defaultShardSize
					sn.NetworkName = container.AppNetworkName()
					sn.MigrationDir = c.String("directory")
					wg.Add(1)
					go func(sn *db.SQLNode) {
						defer wg.Done()
						createNode(ctx, sn)
					}(sn)
				}
			}
			wg.Wait()
			return nil
		},
	}
}
