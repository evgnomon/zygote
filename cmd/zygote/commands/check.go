// Package commands contains all available commands.
package commands

import (
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

func checkAction(_ *cli.Context) error {
	err := utils.Run("scripts/check")
	if err != nil {
		return err
	}
	return nil
}

// CheckCommand runs the check script in the current repo.
func CheckCommand() *cli.Command {
	return &cli.Command{
		Name:   "check",
		Usage:  "Run check script in the current repo",
		Action: checkAction,
	}
}
