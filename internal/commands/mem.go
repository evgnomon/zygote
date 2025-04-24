// Package commands contains all available commands.
package commands

import (
	"github.com/evgnomon/zygote/internal/mem"
	"github.com/urfave/cli/v2"
)

// MemCommand gets/sets memory values.
func MemCommand() *cli.Command {
	return &cli.Command{
		Name:  "mem",
		Usage: "Get/Set memory values",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:    "replica-index",
				Aliases: []string{"n"},
				Value:   0,
				Usage:   "Replica ID, starting 0",
			},
			&cli.Int64Flag{
				Name:    "shard-index",
				Aliases: []string{"s"},
				Value:   0,
				Usage:   "Shared index, starting 0",
			},
			&cli.StringFlag{
				Name:    "domain",
				Aliases: []string{"d"},
				Usage:   "The domain name, e.g. foo.com or foo.bar.com",
				Value:   "zygote.run",
			},
		},
		Action: func(_ *cli.Context) error {
			mem.RunExample()
			return nil
		},
	}
}
