// Package commands contains all available commands.
package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/urfave/cli/v2"
)

// QCommand executes SQL query over HTTPs.
func QCommand() *cli.Command {
	return &cli.Command{
		Name:  "q",
		Usage: "Execute SQL query over HTTPs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "user",
				Aliases: []string{"u"},
				Usage:   "User name with sign certificate",
				Value:   utils.User(),
			},
			&cli.BoolFlag{
				Name:  "curl",
				Usage: "Print curl command instead of executing the query",
			},
		},
		Action: func(c *cli.Context) error {
			server := c.Args().Get(0)
			if server == "" {
				return fmt.Errorf("valid host is required")
			}
			if !strings.Contains(server, ":") {
				server = fmt.Sprintf("%s:443", server)
			}
			url := fmt.Sprintf("https://%s/sql/query", server)
			query, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %v", err)
			}

			certService, err := cert.Cert()
			if err != nil {
				return fmt.Errorf("failed to get certificate service: %v", err)
			}

			// Get user and construct certificate paths using CertService
			user := c.String("user")
			certPath := certService.FunctionCertFile(user)
			keyPath := filepath.Join(certService.FunctionsCertDir(user), fmt.Sprintf("%s_key.pem", user))
			caCertPath := certService.CaCertFileForDomain(server)

			// If curl flag is set, print the curl command and return
			if c.Bool("curl") {
				curlCmd := fmt.Sprintf(`curl -s -X POST \
  --cert %s \
  --key %s \
  --cacert %s \
  -H "Content-Type: application/json" \
  -d '{"query": "%s"}' \
  %s`, certPath, keyPath, caCertPath, strings.ReplaceAll(string(query), `"`, `\"`), url)

				fmt.Println(curlCmd)
				return nil
			}

			// Original execution path
			p := controller.SQLQueryRequest{
				Query: string(query),
			}
			return sendAndPrint(url, server, user, p)
		},
	}
}
