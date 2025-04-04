package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/evgnomon/zygote/pkg/tables"
	"github.com/urfave/cli/v2"
)

func Query() *cli.Command {
	return &cli.Command{
		Name:  "query",
		Usage: "Execute SQL queries from stdin and output results as JSON",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "database",
				Aliases: []string{"d"},
				Usage:   "Database name",
			},
		},
		Action: func(c *cli.Context) error {
			// Initialize database configuration from flags
			config := tables.NewClientConfig()

			if c.String("database") != "" {
				config.Database = c.String("database")
			}

			var connector tables.DatabaseConnector = config
			db, err := connector.Connect()
			if err != nil {
				fmt.Fprintf(os.Stderr, "%v\n", err)
				os.Exit(1)
			}
			defer db.Close()

			if err = db.Ping(); err != nil {
				fmt.Fprintf(os.Stderr, "Database ping failed: %v\n", err)
				os.Exit(1)
			}

			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				query := strings.TrimSpace(scanner.Text())
				if query == "" {
					continue
				}

				rows, err := db.Query(query)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Query execution failed: %v\n", err)
					os.Exit(1)
				}

				columns, err := rows.Columns()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to get columns: %v\n", err)
					os.Exit(1)
				}

				var results []map[string]any
				for rows.Next() {
					values := make([]any, len(columns))
					valuePtrs := make([]any, len(columns))
					for i := range values {
						valuePtrs[i] = &values[i]
					}

					if err := rows.Scan(valuePtrs...); err != nil {
						fmt.Fprintf(os.Stderr, "Failed to scan row: %v\n", err)
						os.Exit(1)
					}

					row := make(map[string]any)
					for i, col := range columns {
						var v any
						if values[i] != nil {
							switch va := values[i].(type) {
							case []byte:
								v = string(va)
							default:
								v = values[i]
							}
						}
						row[col] = v
					}
					results = append(results, row)
				}

				if err = rows.Err(); err != nil {
					fmt.Fprintf(os.Stderr, "Row iteration error: %v\n", err)
					os.Exit(1)
				}

				jsonData, err := json.MarshalIndent(results, "", "  ")
				if err != nil {
					fmt.Fprintf(os.Stderr, "JSON marshaling failed: %v\n", err)
					os.Exit(1)
				}

				fmt.Println(string(jsonData))
				rows.Close()
			}

			if err := scanner.Err(); err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}

			return nil // Successful execution
		},
	}
}
