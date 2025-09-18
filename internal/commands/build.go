// Package commands contains all available commands.
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// YCommand builds the current project.
func YCommand() *cli.Command {
	return &cli.Command{
		Name:    "build",
		Aliases: []string{"y"},
		Usage:   "Build the current project",
		Action: func(_ *cli.Context) error {
			os.Setenv("ANSIBLE_STDOUT_CALLBACK", "yaml")
			if os.Getenv("YACHT_EVENT_NAME") == "" {
				os.Setenv("YACHT_EVENT_NAME", "push")
			}
			refName, err := utils.RunCapture("git", "symbolic-ref", "--short", "HEAD")
			if err != nil {
				return fmt.Errorf("failed to get ref name: %w", err)
			}

			if os.Getenv("YACHT_REF_NAME") == "" {
				os.Setenv("YACHT_REF_NAME", refName)
			}

			err = utils.CreateRepoVault()
			if err != nil {
				return fmt.Errorf("failed to create repo vault: %w", err)
			}

			os.Setenv("ANSIBLE_VAULT_PASSWORD_FILE", filepath.Join(utils.UserHome(), ".config", "zygote", "scripts", "vault_pass"))

			vaultPath := utils.RepoVaultPath()
			if err != nil {
				return err
			}

			secretFilePathExist := utils.PathExists(vaultPath)
			if !secretFilePathExist {
				err = utils.Run("ansible-vault", "create", vaultPath)
				if err != nil {
					return err
				}
			}

			os.Setenv("INPUT_VAULT_FILE", vaultPath)
			err = utils.Run("ansible-playbook", "playbooks/main.yaml")
			if err != nil {
				return err
			}

			return nil
		},
	}
}
