// Package commands contains all available commands.
package commands

import (
	"fmt"
	"strings"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// DeinitCommand releases resources associated with the current repo.
func DeinitCommand() *cli.Command {
	return &cli.Command{
		Name:  "deinit",
		Usage: "Release resources associated with the current repo",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "remove-volume",
				Aliases: []string{"v"},
				Usage:   "Remove volumes associated with the containers",
			},
			&cli.BoolFlag{
				Name:    "yes",
				Aliases: []string{"y"},
				Usage:   "Remove volumes associated with the containers",
			},
		},
		Action: func(c *cli.Context) error {
			if !c.Bool("yes") && !utils.GetYesNoInput("This will remove all containers and optionally volumes.") {
				return nil
			}
			for _, v := range container.List() {
				if strings.HasPrefix(v.Name, fmt.Sprintf("/%s-", utils.TenantName())) {
					container.RemoveContainer(v.ID)
				}
			}
			if c.Bool("remove-volume") {
				container.RemoveVolumePrefix(fmt.Sprintf("%s-", utils.TenantName()))
			}
			return nil
		},
	}
}
