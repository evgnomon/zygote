// Package commands contains all available commands.
package commands

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/evgnomon/zygote/pkg/http"
	resty "github.com/go-resty/resty/v2"
	"github.com/urfave/cli/v2"
)

// CallCommand calls a URL using a client certificate.
func CallCommand() *cli.Command {
	return &cli.Command{
		Name:  "call",
		Usage: "Call a URL using a client certificate",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "url",
				Aliases: []string{"u"},
				Usage:   "URL to call",
			},
			&cli.StringFlag{
				Name:    "method",
				Aliases: []string{"m"},
				Usage:   "HTTP method (GET, POST, PUT, DELETE, etc.)",
				Value:   "GET", // Default to GET
			},
			&cli.StringFlag{
				Name:  "content-type",
				Usage: "Content type for the request",
				Value: "application/json", // Default to JSON
			},
		},
		Action: func(c *cli.Context) error {
			u := c.String("url")
			method := strings.ToUpper(c.String("method"))
			contentType := c.String("content-type")
			hc := http.NewHTTPTransportConfig()
			client, err := hc.Client()
			if err != nil {
				return err
			}

			req := client.R()

			// Handle payload for POST and PUT from stdin
			if method == "POST" || method == "PUT" {
				// Read from stdin
				payload, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("failed to read from stdin: %v", err)
				}
				if len(payload) > 0 {
					req = req.SetBody(payload).
						SetHeader("Content-Type", contentType)
				}
			}

			// Execute the request based on method
			var r *resty.Response
			switch method {
			case "GET":
				r, err = req.Get(u)
			case "POST":
				r, err = req.Post(u)
			case "PUT":
				r, err = req.Put(u)
			case "DELETE":
				r, err = req.Delete(u)
			default:
				return fmt.Errorf("unsupported HTTP method: %s", method)
			}

			if err != nil {
				return err
			}

			fmt.Print(r.String())
			return nil
		},
	}
}
