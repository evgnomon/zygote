// Package commands contains all available commands.
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/pkg/http"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/mattn/go-shellwords"
	"github.com/urfave/cli/v2"
)

// CCommand executes Mem query on a shard.
func CCommand() *cli.Command {
	return &cli.Command{
		Name:  "c",
		Usage: "Execute Mem query on a shard",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "user",
				Aliases: []string{"u"},
				Usage:   "User name with sign certificate",
				Value:   utils.User(),
			},
		},
		Action: func(c *cli.Context) error {
			server := c.Args().Get(0)
			if server == "" {
				server = "zygote:8443"
			}
			if !strings.Contains(server, ":") {
				server = fmt.Sprintf("%s:443", server)
			}
			url := fmt.Sprintf("https://%s/mem/query", server)
			query, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read from stdin: %v", err)
			}
			if len(query) == 0 {
				return fmt.Errorf("query cannot be empty")
			}

			parser := shellwords.NewParser()
			parts, err := parser.Parse(string(query))
			if err != nil {
				return fmt.Errorf("failed to parse query: %v", err)
			}
			p := controller.RedisQueryRequest{
				Query: parts,
			}
			return sendAndPrint(url, server, c.String("user"), p)
		},
	}
}

func sendAndPrint(url, server, user string, p any) error {
	// json.Marshal will handle all necessary escaping
	payload, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %v", err)
	}
	domain := strings.Split(server, ":")[0]
	t := http.NewHTTPTransportConfigForUserHost(user, domain)

	client, err := t.Client()
	if err != nil {
		return err
	}

	r, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(payload).
		Post(url)
	fmt.Print(r.String())
	if err != nil {
		return err
	}
	return nil
}
