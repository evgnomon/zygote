// Package commands contains all available commands.
package commands

import (
	"fmt"
	"os"
	"syscall"

	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

const routerReadWritePort = 6446
const routerReadOnlyPort = 7447

// SQLCommand provides a SQL shell to interact with the database.
func SQLCommand() *cli.Command {
	return &cli.Command{
		Name:  "sql",
		Usage: "SQL shell to interact with the database",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "s",
				Usage: "shard index",
				Value: 0,
			},
			&cli.StringFlag{
				Name:  "i",
				Usage: "Input script",
			},
			&cli.BoolFlag{
				Name:    "r",
				Aliases: []string{"read-only"},
				Usage:   "Connect through read-only port",
			},
			&cli.StringFlag{
				Name:    "user",
				Aliases: []string{"u"},
				Usage:   "Connect as a user",
			},
			&cli.StringFlag{
				Name:  "host",
				Usage: "Host name",
				Value: "127.0.0.1",
			},
		},
		Action: func(c *cli.Context) error {
			user := "root"
			password := "root1234"
			if c.String("user") != "" {
				fmt.Print("Enter password: ")
				bytePassword, err := term.ReadPassword(int(syscall.Stdin)) //nolint:unconvert
				if err != nil {
					return fmt.Errorf("failed to read password: %w", err)
				}
				password = string(bytePassword)
				user = c.String("user")
				fmt.Println()
			}

			// Construct the MySQL DSN (Data Source Name)
			n := c.Int("s")
			os.Setenv("MYSQL_PWD", password)
			portNum := fmt.Sprintf("%d", routerReadWritePort+100*n)
			if c.Bool("read-only") {
				portNum = fmt.Sprintf("%d", routerReadOnlyPort+100*n)
			}

			command := []string{"mysql", "-u", user, "-h", c.String("host"), "-P", portNum, "-s", "--auto-rehash"}
			if c.String("i") != "" {
				command = append(command, "-e", c.String("i"))
			}
			err := utils.Run(command...)
			if err != nil {
				return err
			}
			return nil
		},
	}
}
