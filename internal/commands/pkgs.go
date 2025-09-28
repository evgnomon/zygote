/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <https://evgnomon.org/docs/hgl>
*/
package commands

import (
	"fmt"

	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// OpenDiffs opens diffs in the browser.
func OpenPackages() *cli.Command {
	return &cli.Command{
		Name:  "pkgs",
		Usage: "Open diffs in the browser",
		Action: func(_ *cli.Context) error {
			return utils.OpenURLInBrowser(fmt.Sprintf("https://github.com/users/%s/packages/container/package/%s", utils.User(), utils.RepoName()))
		},
	}
}
