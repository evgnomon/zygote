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
	"sync"
	"syscall"
	"time"

	resty "github.com/go-resty/resty/v2"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/mattn/go-shellwords"
	cli "github.com/urfave/cli/v2"
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

const httpClientTimeout = 10 * time.Second
const editor = "vi"
const varChar255 = "VARCHAR(255)"
const dirPerm = 0755
const routerReadWritePort = 6446
const routerReadOnlyPort = 7447
const defaultShardSize = 3
const defaultNumShards = 3

var logger = util.NewLogger()

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
			sqlDirExists := utils.PathExists(dir)
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
					m := migration.NewMigration(c.String("directory"))
					return m.Up(ctx)
				},
			},
			{
				Name:  "down",
				Usage: "Revert the last applied migration to undo changes to the database schema.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := migration.NewMigration(c.String("directory"))
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
		},
		Action: func(c *cli.Context) error {
			ctx := context.Background()
			var wg sync.WaitGroup

			for shardIndex := 0; shardIndex < defaultNumShards; shardIndex++ {
				for repIndex := 0; repIndex < defaultShardSize; repIndex++ {
					sn := db.NewSQLNode()
					sn.Domain = "my.zygote.run"
					sn.DatabaseName = "zygote"
					sn.Tenant = "zygote"
					sn.ShardIndex = shardIndex
					sn.RepIndex = repIndex
					sn.NumShards = defaultNumShards
					sn.ShardSize = defaultShardSize
					sn.NetworkName = container.AppNetworkName()
					sn.MigrationDir = c.String("directory")
					wg.Add(1)
					go func(sn *db.SQLNode) {
						defer wg.Done()
						createNode(ctx, sn)
					}(sn)
				}
			}
			wg.Wait()
			return nil
		},
	}
}

func createNode(ctx context.Context, sn *db.SQLNode) {
	n := container.NewNetworkConfig(sn.NetworkName)
	n.Ensure(ctx)
	err := sn.StartSQLContainers(ctx)
	logger.FatalIfErr("Make SQL node", err)
	mc := mem.NewMemNode()
	mc.Domain = sn.Domain
	mc.Tenant = sn.Tenant
	mc.ShardIndex = sn.ShardIndex
	mc.ReplicaIndex = sn.RepIndex
	mc.NetworkName = sn.NetworkName
	if mc.NetworkName != container.HostNetworkName {
		mc.NetworkName = container.AppNetworkName()
	}
	mc.ShardSize = sn.ShardSize
	mc.NumShards = sn.NumShards
	mc.CreateReplica(ctx)
	mc.Init(ctx)
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
			ctx := context.Background()
			var sn db.SQLNode
			sn.Domain = c.String("domain")
			sn.NetworkName = container.HostNetworkName
			sn.DatabaseName = c.String("db")
			h, err := util.CalculateIndices(c.String("host"))
			if err != nil {
				return err
			}
			sn.ShardIndex = h.ShardIndex
			sn.RepIndex = h.RepIndex
			sn.NumShards = c.Int("num-shards")
			sn.ShardSize = c.Int("shard-size")

			createNode(ctx, &sn)
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
				name := utils.RepoFullName()
				dbName = name
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
				name := utils.RepoFullName()
				dbName = name
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
				name := utils.RepoFullName()
				dbName = name
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
				name := utils.RepoFullName()
				dbName = name
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
				name := utils.RepoFullName()
				dbName = name
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
			if utils.PathExists(tempDir) {
				err = os.RemoveAll(tempDir)
				if err != nil {
					return fmt.Errorf("failed to remove /tmp/smoker: %w", err)
				}
			}
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
				logger.FatalIfErr("Change back to original directory", err)
			}()

			defer func() {
				err = utils.Script([][]string{{"sudo", zygotePath, "deinit", "-v"}})
				logger.FatalIfErr("Deinit zygote", err)
			}()
			err = utils.Script([][]string{
				{"rm", "-rf", "./sqls"},
				{"sudo", zygotePath, "deinit", "-v"},
				{"sudo", zygotePath, "init"},
				{"sudo", "-K"},
				{zygotePath, "gen", "db", "--name=smokers"},
				{zygotePath, "gen", "table", "--name=users", "--db=smokers"},
				{zygotePath, "gen", "table", "--name=posts", "--db=smokers"},
				{zygotePath, "gen", "table", "--name=comments", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=name", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=email", "--type=string", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=users", "--name=email", "--col=email", "-u", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=users", "--name", "users_name", "--col", "email", "--col", "name", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=active", "--type=bool", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=pic", "--type=binary", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=users", "--name=age", "--type=double", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=title", "--type=string", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=uuid", "--type=uuid", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=content", "--type=text", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=views", "--type=integer", "--db=smokers"},
				{zygotePath, "gen", "col", "--table=posts", "--name=tags", "--type=json", "--db=smokers"},
				{zygotePath, "gen", "prop", "--table=posts", "--name=tag", "--type=string", "--path=$.name", "--db=smokers"},
				{zygotePath, "gen", "index", "--table=posts", "--name=tags", "--col=tag", "--col=uuid", "--db=smokers"},
				{zygotePath, "gen", "index", "-t", "posts", "-n", "content", "--col=content", "--full-text", "--db=smokers"},
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
	logger.FatalIfErr("Write scripts", err)
	logger := util.NewLogger()

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
		logger.Error("error running command", err)
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
