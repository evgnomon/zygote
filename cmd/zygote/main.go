/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <http://evgnomon.org/docs/hgl>
*/
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	resty "github.com/go-resty/resty/v2"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mattn/go-shellwords"
	cli "github.com/urfave/cli/v2"
	"go.uber.org/zap"
	"golang.org/x/term"

	"github.com/evgnomon/zygote/cmd/zygote/commands"
	"github.com/evgnomon/zygote/internal/cert"
	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/controller"
	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/internal/mem"
	"github.com/evgnomon/zygote/internal/migration"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/utils"
)

const containerStartTimeout = 20 * time.Second
const httpClientTimeout = 10 * time.Second
const redisRetryInterval = 10 * time.Second
const editor = "vi"
const varChar255 = "VARCHAR(255)"
const dirPerm = 0755
const routerReadWritePort = 16446
const routerReadOnlyPort = 17447
const defaultShardSize = 3
const mysqlRouterConfTmplName = "router.conf"
const maxMemClusterCreateRetry = 5

func vaultCommand() *cli.Command {
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

func blueprintCommand() *cli.Command {
	return &cli.Command{
		Name:    "blueprint",
		Aliases: []string{"x"},
		Usage:   "Setup this machine with Blueprint. Upgrade the machine if it is alreadt been setup",
		Action: func(_ *cli.Context) error {
			currentDir, err := os.Getwd()
			if err != nil {
				return err
			}
			err = utils.Elevate()
			if err != nil {
				return err
			}
			err = utils.Chdir(fmt.Sprintf("%s/src/github.com/%s/blueprint", utils.UserHome(), utils.User()))
			if err != nil {
				return err
			}
			err = utils.Run("ansible-playbook", "-i", "inventory.py", "main.yaml")
			if err != nil {
				return err
			}
			err = utils.Chdir(currentDir)
			if err != nil {
				return err
			}
			err = utils.UnElevate()
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func buildCommand() *cli.Command {
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

			vaultPath, err := utils.RepoVaultPath()
			if err != nil {
				return err
			}

			secretFilePathExist, err := utils.PathExists(vaultPath)
			if err != nil {
				return err
			}
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

func checkAction(_ *cli.Context) error {
	err := utils.Run("scripts/check")
	if err != nil {
		return err
	}
	return nil
}

func checkCommand() *cli.Command {
	return &cli.Command{
		Name:   "check",
		Usage:  "Run check script in the current repo",
		Action: checkAction,
	}
}

func migrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Manage database migrations. Allows you to apply or revert changes to the database schema.",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "directory",
				Aliases: []string{"C"},
				Value:   "sqls",
				Usage:   "Directory containing the SQL migration files.",
			},
		},
		Action: func(c *cli.Context) error {
			dir := c.String("directory")
			if dir == "" {
				dir = "sqls"
			}
			sqlDirExists, err := utils.PathExists(dir)
			if err != nil {
				return fmt.Errorf("failed to check if directory exists: %w", err)
			}
			if !sqlDirExists {
				return nil
			}
			return nil
		},
		Subcommands: []*cli.Command{
			{
				Name:  "up",
				Usage: "Apply all pending migrations to update the database schema to the latest version.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := NewMigration(c.String("directory"))
					return m.Up(ctx)
				},
			},
			{
				Name:  "down",
				Usage: "Revert the last applied migration to undo changes to the database schema.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := NewMigration(c.String("directory"))
					return m.Down(ctx)
				},
			},
		},
	}
}

func initCommand() *cli.Command {
	return &cli.Command{
		Name:  "init",
		Usage: "Initialize resources for the current repo",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "directory",
				Aliases: []string{"C"},
				Value:   "sqls",
				Usage:   "Directory containing the SQL migration files.",
			},
			&cli.BoolFlag{
				Name:    "local",
				Aliases: []string{"l"},
				Value:   true,
				Usage:   "Initialize resources using a local instance of Zygote core",
			},
		},
		Action: func(c *cli.Context) error {
			ctx := context.Background()
			if c.Bool("local") {
				err := initSQLClusterLocal(ctx, c.String("directory"))
				return err
			}
			err := initContainers(ctx, c.String("directory"))
			return err
		},
	}
}

func joinCommand() *cli.Command {
	return &cli.Command{
		Name:  "join",
		Usage: "Join to a remote cluster",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "directory",
				Aliases: []string{"C"},
				Value:   "sqls",
				Usage:   "Directory containing the SQL migration files.",
			},
			&cli.IntFlag{
				Name:  "num-shards",
				Value: 1,
				Usage: "number of shards",
			},
			&cli.IntFlag{
				Name:  "shard-size",
				Value: 3,
				Usage: "number of nodes in a shard",
			},
			&cli.StringFlag{
				Name:     "domain",
				Aliases:  []string{"d"},
				Usage:    "The domain name, e.g. foo.com or foo.bar.com",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "host",
				Usage:    "The host name, e.g. shard-a-1.foo.com or shard-b.foo.bar.com",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			_, err := util.Logger()
			if err != nil {
				return err
			}
			ctx := context.Background()
			var cl db.SQLShard
			cl.Domain = c.String("domain")
			cl.NetworkName = "host"
			cl.DatabaseName = c.String("db")

			h, err := util.CalculateIndices(c.String("host"))
			if err != nil {
				return err
			}

			err = cl.Create(ctx, h.ShardIndex, h.RepIndex)
			if err != nil {
				return fmt.Errorf("failed to create shard: %w", err)
			}
			mc := mem.NewMemShard(cl.Domain)
			mc.NumShards = c.Int("num-shards")
			mc.ShardSize = c.Int("shard-size")
			err = mc.CreateReplica(h.RepIndex)
			if err != nil {
				return fmt.Errorf("failed to create replica: %w", err)
			}
			count := 0
			logging, err := util.Logger()
			if err != nil {
				return fmt.Errorf("failed to create logger: %w", err)
			}
			if h.RepIndex == 0 && c.Int("shard-index") == 0 {
				for count < maxMemClusterCreateRetry {
					err = mc.Init(ctx)
					if err != nil {
						logging.Warn("failed to init replica", zap.Error(err))
						time.Sleep(redisRetryInterval)
						count++
						continue
					}
					break
				}
			}
			return nil
		},
	}
}

func memCommand() *cli.Command {
	return &cli.Command{
		Name:  "mem",
		Usage: "Get/Set memory values",
		Flags: []cli.Flag{
			&cli.Int64Flag{
				Name:    "replica-index",
				Aliases: []string{"n"},
				Value:   0,
				Usage:   "Replica ID, starting 0",
			},
			&cli.Int64Flag{
				Name:    "shard-index",
				Aliases: []string{"s"},
				Value:   0,
				Usage:   "Shared index, starting 0",
			},
			&cli.StringFlag{
				Name:    "domain",
				Aliases: []string{"d"},
				Usage:   "The domain name, e.g. foo.com or foo.bar.com",
				Value:   "zygote.run",
			},
		},
		Action: func(_ *cli.Context) error {
			mem.RunExample()
			return nil
		},
	}
}

func deinitCommand() *cli.Command {
	return &cli.Command{
		Name:  "deinit",
		Usage: "Release resources associated with the current repo",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "remove-volume",
				Aliases: []string{"v"},
				Usage:   "Remove volumes associated with the containers",
			},
		},
		Action: func(c *cli.Context) error {
			for _, v := range container.List() {
				if strings.HasPrefix(v.Name, "/zygote-") {
					container.RemoveContainer(v.ID)
				}
			}
			if c.Bool("remove-volume") {
				container.RemoveVolumePrefix("zygote-")
			}
			return nil
		},
	}
}

func runCommand() *cli.Command {
	return &cli.Command{
		Name:  "run",
		Usage: "Run the application",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "init",
				Usage: "Remove volumes associated with the containers",
			},
			&cli.StringFlag{
				Name:    "directory",
				Aliases: []string{"C"},
				Value:   "sqls",
				Usage:   "Directory containing the SQL migration files.",
			},
		},
		Action: func(c *cli.Context) error {
			for _, v := range container.List() {
				if strings.HasPrefix(v.Name, "/zygote-") {
					container.RemoveContainer(v.ID)
				}
			}
			if c.Bool("init") {
				container.RemoveVolumePrefix("zygote-")
			}
			err := initContainers(context.Background(), c.String("directory"))
			return err
		},
	}
}

func certCommand() *cli.Command {
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
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Domain address",
						Value:   utils.User(),
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

					return cs.Sign([]string{c.String("name")}, time.Now().AddDate(1, 0, 0), c.String("password"))
				},
			},
		},
	}
}

func callCommand() *cli.Command {
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
			hc := NewHTTPTransportConfig()
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

func qCommand() *cli.Command {
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

func cCommand() *cli.Command {
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
	t := NewHTTPTransportConfigForUserHost(user, domain)

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

func openDiffs() *cli.Command {
	return &cli.Command{
		Name:  "diffs",
		Usage: "Open diffs in the browser",
		Action: func(_ *cli.Context) error {
			return utils.OpenURLInBrowser("https://github.com/pulls")
		},
	}
}

func openActions() *cli.Command {
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

func sqlCommand() *cli.Command {
	return &cli.Command{
		Name:  "sql",
		Usage: "SQL shell to interact with the database",
		Flags: []cli.Flag{
			&cli.IntFlag{
				Name:  "n",
				Usage: "Instance number",
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
			n := c.Int("n")
			os.Setenv("MYSQL_PWD", password)
			dbName, err := utils.RepoFullName()
			if err != nil {
				return fmt.Errorf("failed to get repo full name: %w", err)
			}

			portNum := fmt.Sprintf("%d", 3306+n-1)
			if n == 0 {
				portNum = fmt.Sprintf("%d", routerReadWritePort)
			}
			if c.Bool("read-only") {
				portNum = fmt.Sprintf("%d", routerReadOnlyPort)
			}

			command := []string{"mysql", "-u", user, "-h", "127.0.0.1", "-P", portNum, "-s", "--auto-rehash", "-D", dbName}
			if c.String("i") != "" {
				command = append(command, "-e", c.String("i"))
			}
			err = utils.Run(command...)
			if err != nil {
				return err
			}
			return nil
		},
	}
}

func sqlCol(colType string) string {
	var sqlCol string
	switch colType {
	case "string":
		sqlCol = varChar255
	case "integer":
		sqlCol = "INT"
	case "double":
		sqlCol = "DOUBLE"
	case "bool":
		sqlCol = "BOOLEAN"
	case "binary":
		sqlCol = "MEDIUMBLOB"
	case "json":
		sqlCol = "JSON"
	case "uuid":
		sqlCol = "CHAR(36)"
	case "text":
		sqlCol = "MEDIUMTEXT"
	default:
		sqlCol = varChar255
	}
	return sqlCol
}

func generateDBCommand() *cli.Command {
	return &cli.Command{
		Name:  "db",
		Usage: "Create a database",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "name",
				Aliases: []string{"n"},
				Usage:   "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			dbName := c.String("name")
			if dbName == "" {
				name, err := utils.RepoFullName()
				dbName = name
				if err != nil {
					return fmt.Errorf("failed to get repo full name: %w", err)
				}
			}
			m, err := db.CreateDatabase(dbName)
			if err != nil {
				return fmt.Errorf("failed to create database: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateTableCommand() *cli.Command {
	return &cli.Command{
		Name:    "table",
		Usage:   "Create a table",
		Aliases: []string{"tab"},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
		},
		Action: func(c *cli.Context) error {
			tableName := c.String("name")
			if tableName == "" {
				return fmt.Errorf("name is required")
			}
			dbName := c.String("db")
			if dbName == "" {
				name, err := utils.RepoFullName()
				dbName = name
				if err != nil {
					return fmt.Errorf("failed to get repo full name: %w", err)
				}
			}

			m, err := db.CreateTable(dbName, tableName)
			if err != nil {
				return fmt.Errorf("failed to create table: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateColCommand() *cli.Command {
	return &cli.Command{
		Name:    "column",
		Aliases: []string{"col"},
		Usage:   "Create a column",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the column",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.StringFlag{
				Name:     "table",
				Usage:    "Name of the table",
				Aliases:  []string{"t"},
				Required: true,
			},
			&cli.StringFlag{
				Name:  "type",
				Usage: "Column type: string, integer, double, bool, binary, json, text, uuid",
				Value: "string",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name, err := utils.RepoFullName()
				dbName = name
				if err != nil {
					return fmt.Errorf("failed to get repo full name: %w", err)
				}
			}

			colType := c.String("type")
			if colType == "" {
				colType = "string"
			}

			m, err := db.CreateColumn(dbName, tableName, colName, sqlCol(colType))
			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generatePropCommand() *cli.Command {
	return &cli.Command{
		Name:    "property",
		Aliases: []string{"prop"},
		Usage:   "Extract a field out of a JSON and store it in a new column",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the column",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "path",
				Usage:    "Field path in the JSON",
				Value:    "string",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "table",
				Aliases:  []string{"t"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.StringFlag{
				Name:  "type",
				Usage: "Column type: string, integer, double, bool, binary, json, text, uuid",
				Value: "string",
			},
			&cli.BoolFlag{
				Name:  "virtual",
				Usage: "Name of the virtual property",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			fieldPath := c.String("path")
			if fieldPath == "" {
				return fmt.Errorf("JSON field path is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name, err := utils.RepoFullName()
				dbName = name
				if err != nil {
					return fmt.Errorf("failed to get repo full name: %w", err)
				}
			}

			m, err := db.CreateProperty(dbName, tableName, colName, fieldPath,
				sqlCol(c.String("type")), c.Bool("virtual"))
			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateIndexCommand() *cli.Command {
	return &cli.Command{
		Name:  "index",
		Usage: "Create an index",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "name",
				Aliases:  []string{"n"},
				Usage:    "Name of the index",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "table",
				Aliases:  []string{"t"},
				Usage:    "Name of the table",
				Required: true,
			},
			&cli.StringSliceFlag{
				Name:     "column",
				Aliases:  []string{"col"},
				Usage:    "Name of the column. Use more than once for multiple columns",
				Required: true,
			},
			&cli.StringFlag{
				Name:  "db",
				Usage: "Name of the database",
			},
			&cli.BoolFlag{
				Name:    "unique",
				Usage:   "Create a unique index",
				Aliases: []string{"u"},
			},
			&cli.BoolFlag{
				Name:  "full-text",
				Usage: "Create a full text index",
			},
		},
		Action: func(c *cli.Context) error {
			colName := c.String("name")
			if colName == "" {
				return fmt.Errorf("name is required")
			}

			tableName := c.String("table")
			if tableName == "" {
				return fmt.Errorf("table is required")
			}

			dbName := c.String("db")
			if dbName == "" {
				name, err := utils.RepoFullName()
				dbName = name
				if err != nil {
					return fmt.Errorf("failed to get repo full name: %w", err)
				}
			}

			if c.Bool("unique") && c.Bool("full-text") {
				return fmt.Errorf("cannot create both unique and full-text index")
			}

			m, err := db.GenCreateSQL(&db.CreateIndexParams{
				CreateSQLParams: db.CreateSQLParams{
					Type:         "index",
					DatabaseName: dbName,
					TableName:    tableName,
					Name:         colName,
				},
				Columns:  c.StringSlice("column"),
				Unique:   c.Bool("unique"),
				FullText: c.Bool("full-text"),
			})

			if err != nil {
				return fmt.Errorf("failed to create column: %w", err)
			}
			err = m.Save()
			if err != nil {
				return fmt.Errorf("failed to save model: %w", err)
			}
			return nil
		},
	}
}

func generateCommand() *cli.Command {
	return &cli.Command{
		Name:    "generate",
		Usage:   "Generate source files",
		Aliases: []string{"gen"},
		Subcommands: []*cli.Command{
			generateDBCommand(),
			generateTableCommand(),
			generateColCommand(),
			generatePropCommand(),
			generateIndexCommand(),
		},
	}
}

func buildZygote() error {
	err := utils.Run("go", "build", "./cmd/zygote")
	if err != nil {
		return fmt.Errorf("failed to build zygote: %w", err)
	}
	return nil
}

func smokerCommand() *cli.Command {
	return &cli.Command{
		Name:  "smoke",
		Usage: "Run smoke tests",
		Action: func(_ *cli.Context) error {
			err := buildZygote()
			if err != nil {
				return fmt.Errorf("failed to build zygote: %w", err)
			}

			zygotePath := filepath.Join(os.Getenv("PWD"), "zygote")

			// Create temporary directory and change to it
			tempDir := "/tmp/smoker"
			err = os.Mkdir(tempDir, dirPerm)
			if err != nil {
				return fmt.Errorf("failed to create /tmp/smoker: %w", err)
			}
			defer os.RemoveAll(tempDir) // Clean up at the end

			err = os.Chdir(tempDir)
			if err != nil {
				return fmt.Errorf("failed to change to /tmp/smoker: %w", err)
			}

			defer func() {
				// Change back to original directory
				err = os.Chdir(os.Getenv("PWD"))
				if err != nil {
					panic(fmt.Errorf("failed to change back to original directory: %w", err))
				}
			}()

			defer func() {
				err = utils.Script([][]string{{"sudo", zygotePath, "deinit", "-v"}})
				if err != nil {
					panic(fmt.Errorf("failed to run smoke tests: %w", err))
				}
			}()
			err = utils.Script([][]string{
				{"rm", "-rf", "./sqls"},
				{"sudo", zygotePath, "deinit", "-v"},
				{"sudo", zygotePath, "init"},
				{"sudo", "-K"},
				{zygotePath, "gen", "db", "--name=smokers"},
				{zygotePath, "gen", "table", "--name=users"},
				{zygotePath, "gen", "table", "--name=posts"},
				{zygotePath, "gen", "table", "--name=comments"},
				{zygotePath, "gen", "col", "--table=users", "--name=name"},
				{zygotePath, "gen", "col", "--table=users", "--name=email", "--type=string"},
				{zygotePath, "gen", "index", "--table=users", "--name=email", "--col=email", "-u"},
				{zygotePath, "gen", "index", "--table=users", "--name", "users_name", "--col", "email", "--col", "name"},
				{zygotePath, "gen", "col", "--table=users", "--name=active", "--type=bool"},
				{zygotePath, "gen", "col", "--table=users", "--name=pic", "--type=binary"},
				{zygotePath, "gen", "col", "--table=users", "--name=age", "--type=double"},
				{zygotePath, "gen", "col", "--table=posts", "--name=title"},
				{zygotePath, "gen", "col", "--table=posts", "--name=uuid", "--type=uuid"},
				{zygotePath, "gen", "col", "--table=posts", "--name=content", "--type=text"},
				{zygotePath, "gen", "col", "--table=posts", "--name=views", "--type=integer"},
				{zygotePath, "gen", "col", "--table=posts", "--name=tags", "--type=json"},
				{zygotePath, "gen", "prop", "--table=posts", "--name=tag", "--type=string", "--path=$.name"},
				{zygotePath, "gen", "index", "--table=posts", "--name=tags", "--col=tag", "--col=uuid"},
				{zygotePath, "gen", "index", "-t", "posts", "-n", "content", "--col=content", "--full-text"},
				{zygotePath, "migrate", "up"},
				{zygotePath, "migrate", "down"},
				{zygotePath, "migrate", "up"},
				{zygotePath, "migrate", "down"},
				{zygotePath, "migrate", "up"},
			})

			if err != nil {
				return fmt.Errorf("failed to run smoke tests: %w", err)
			}

			return nil
		},
	}
}

func main() {
	os.Setenv("EDITOR", editor)
	err := utils.WriteScripts()
	if err != nil {
		panic(err)
	}

	logger, err := util.Logger()
	if err != nil {
		panic(err)
	}

	app := &cli.App{
		Action: func(_ *cli.Context) error {
			return checkAction(nil)
		},
		Commands: []*cli.Command{
			vaultCommand(),
			blueprintCommand(),
			buildCommand(),
			checkCommand(),
			migrateCommand(),
			initCommand(),
			deinitCommand(),
			runCommand(),
			certCommand(),
			callCommand(),
			qCommand(),
			openDiffs(),
			openActions(),
			sqlCommand(),
			generateCommand(),
			smokerCommand(),
			joinCommand(),
			memCommand(),
			commands.Query(),
			cCommand(),
		},
	}
	if err := app.Run(os.Args); err != nil {
		logger.Error("error running command", zap.Error(err))
	}
}

func NewMigration(directory string) *migration.Migration {
	return &migration.Migration{
		Directory: directory,
	}
}

type HTTPTransportConfig struct {
	caCertFile   string
	funcCertFile string
	funcKeyFile  string
}

func NewHTTPTransportConfig() *HTTPTransportConfig {
	cs, err := cert.Cert()
	if err != nil {
		log.Fatalf("Failed to create cert service: %v", err)
	}
	return &HTTPTransportConfig{
		caCertFile:   cs.CaCertFile(),
		funcCertFile: cs.FunctionCertFile(utils.User()),
		funcKeyFile:  cs.FunctionKeyFile(utils.User()),
	}
}

func NewHTTPTransportConfigForUserHost(userName, domain string) *HTTPTransportConfig {
	cs, err := cert.Cert()
	if err != nil {
		log.Fatalf("Failed to create cert service: %v", err)
	}
	return &HTTPTransportConfig{
		caCertFile:   cs.CaCertFileForDomain(domain),
		funcCertFile: cs.FunctionCertFile(userName),
		funcKeyFile:  cs.FunctionKeyFile(userName),
	}
}

func (s *HTTPTransportConfig) Client() (*resty.Client, error) {
	serverCACert, err := os.ReadFile(s.caCertFile) // The CA that signed the server's certificate
	if err != nil {
		return nil, err
	}
	systemCAs, err := x509.SystemCertPool()
	if err != nil {
		return nil, err
	}
	serverCAs := systemCAs.Clone()
	if ok := serverCAs.AppendCertsFromPEM(serverCACert); !ok {
		return nil, fmt.Errorf("failed to append server CA cert")
	}

	clientCert, err := tls.LoadX509KeyPair(s.funcCertFile, s.funcKeyFile)
	if err != nil {
		return nil, err
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      serverCAs,
		MinVersion:   tls.VersionTLS12,
	}

	client := resty.New()
	client.SetTransport(&http.Transport{
		TLSClientConfig: tlsConfig,
	})

	client.SetTimeout(httpClientTimeout)
	return client, nil
}

func initContainers(ctx context.Context, directory string) error {
	numShards := 2
	dbName, err := utils.RepoFullName()
	if err != nil {
		return fmt.Errorf("failed to get repo full name: %w", err)
	}
	for i := 1; i <= numShards; i++ {
		sqlParams := container.SQLInitParams{
			DBName:   dbName,
			Username: "admin",
			Password: "password",
		}
		sqlStatements, err := container.ApplyTemplate("sql_init_template.sql", sqlParams)
		if err != nil {
			return err
		}

		container.Vol(sqlStatements, fmt.Sprintf("zygote-db-conf-%d", i), "/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())
	}
	db.CreateDBContainer(2, container.AppNetworkName())
	mem.CreateMemContainer(3, container.AppNetworkName())
	container.InitRedisCluster()
	container.WaitHealthy("zygote-", containerStartTimeout)
	m := NewMigration(directory)
	return m.Up(ctx)
}

func initSQLClusterLocal(ctx context.Context, directory string) error {
	dbName, err := utils.RepoFullName()
	if err != nil {
		return fmt.Errorf("failed to get repo full name: %w", err)
	}
	for i := 1; i <= defaultShardSize; i++ {
		clusterParams := container.InnoDBClusterParams{
			ServerID:             i,
			GroupReplicationPort: 33061,
			ServerCount:          3,
			ServersList:          "zygote-db-rep-1:33061,zygote-db-rep-2:33061,zygote-db-rep-3:33061",
			ReportAddress:        fmt.Sprintf("zygote-db-rep-%d", i),
			ReportPort:           3306 + i - 1,
		}
		innodbGroupReplication, err := container.ApplyTemplate("innodb_cluster_template.cnf", clusterParams)
		if err != nil {
			return err
		}
		sqlParams := container.SQLInitParams{
			DBName:   dbName,
			Username: "admin",
			Password: "password",
		}
		sqlStatements, err := container.ApplyTemplate("sql_init_template.sql", sqlParams)
		if err != nil {
			return err
		}
		routerConfParams := container.RouterConfParams{
			Destinations: "zygote-db-rep-1:3306,zygote-db-rep-2:3306,zygote-db-rep-3:3306",
		}
		routerConf, err := container.ApplyTemplate(mysqlRouterConfTmplName, routerConfParams)
		if err != nil {
			return err
		}
		container.Vol(sqlStatements, fmt.Sprintf("zygote-db-conf-%d", i), "/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())

		container.Vol(innodbGroupReplication, fmt.Sprintf("zygote-db-conf-gr-%d", i), "/etc/mysql/conf.d/", "gr.cnf", container.AppNetworkName())

		container.Vol(routerConf, fmt.Sprintf("zygote-db-router-conf-%d", i), "/etc/mysqlrouter/", "router.conf", container.AppNetworkName())
	}
	db.CreateGroupReplicationContainer(3, container.AppNetworkName())
	container.WaitHealthy("zygote-", containerStartTimeout)
	db.SetupGroupReplication()
	db.CreateRouter(0, container.AppNetworkName())
	container.WaitHealthy("zygote-", containerStartTimeout)
	mem.CreateMemContainer(3, container.AppNetworkName())
	container.InitRedisCluster()
	m := NewMigration(directory)
	return m.Up(ctx)
}
