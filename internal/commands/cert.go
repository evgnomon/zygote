// Package commands contains all available commands.
package commands

import (
	"fmt"
	"time"

	"github.com/evgnomon/zygote/pkg/cert"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// CertCommand manages certificates.
func CertCommand() *cli.Command {
	return &cli.Command{
		Name:  "cert",
		Usage: "Certificate management",
		Subcommands: []*cli.Command{
			{
				Name:  "root",
				Usage: "Create a self-signed certificate or validate an existing one",
				Flags: []cli.Flag{
					&cli.Int64Flag{
						Name:    "days",
						Value:   365,
						Aliases: []string{"c"},
						Usage:   "Number of days the certificate is valid for",
					},
				},
				Action: func(c *cli.Context) error {
					cs, err := cert.Cert()
					if err != nil {
						return err
					}
					return cs.MakeRootCert(time.Now().AddDate(0, 0, c.Int("days")))
				},
			},
			{
				Name:  "sign",
				Usage: "Sign a certificate",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Domain address",
						Value:   cli.NewStringSlice(utils.User()),
					},
					&cli.StringFlag{
						Name:    "password",
						Aliases: []string{"p"},
						Usage:   "Password for the certificate",
					},
				},
				Action: func(c *cli.Context) error {
					cs, err := cert.Cert()
					if err != nil {
						return err
					}

					if c.String("name") == "" {
						return fmt.Errorf("name is required")
					}

					return cs.Sign(c.StringSlice("name"), time.Now().AddDate(1, 0, 0), c.String("password"))
				},
			},
		},
	}
}
