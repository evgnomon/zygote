// Package commands contains all available commands.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// OpenActions opens playbook runs in the browser.
func OpenActions() *cli.Command {
	return &cli.Command{
		Name:  "plays",
		Usage: "Open playbook runs in the browser",
		Action: func(_ *cli.Context) error {
			repoDir, err := os.Getwd()
			if err != nil {
				return fmt.Errorf("failed to get current directory: %w", err)
			}
			repoName := filepath.Base(repoDir)

			orgDir := filepath.Dir(repoDir)
			orgName := filepath.Base(orgDir)

			url := fmt.Sprintf("https://github.com/%s/%s/actions", orgName, repoName)
			return utils.OpenURLInBrowser(url)
		},
	}
}
