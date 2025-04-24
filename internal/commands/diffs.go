// Package commands contains all available commands.
package commands

import (
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// OpenDiffs opens diffs in the browser.
func OpenDiffs() *cli.Command {
	return &cli.Command{
		Name:  "diffs",
		Usage: "Open diffs in the browser",
		Action: func(_ *cli.Context) error {
			return utils.OpenURLInBrowser("https://github.com/pulls")
		},
	}
}
