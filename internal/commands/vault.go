// Package commands contains all available commands.
package commands

import (
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// VaultCommand encrypts and decrypts secrets.
func VaultCommand() *cli.Command {
	return &cli.Command{
		Name:  "vault",
		Usage: "Encrypt and Decrypt secrets",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "encrypt",
				Aliases: []string{"e"},
				Usage:   "File to encrypt",
			},
			&cli.StringFlag{
				Name:    "decrypt",
				Aliases: []string{"d"},
				Usage:   "File to decrypt",
			},
		},
		Action: func(c *cli.Context) error {
			if c.String("encrypt") != "" {
				err := utils.EncryptFile(c.String("encrypt"), c.Args().Get(0))
				if err != nil {
					return err
				}
			} else if c.String("decrypt") != "" {
				err := utils.DecryptFile(c.String("decrypt"))
				if err != nil {
					return err
				}
			}

			return nil
		},
	}
}
