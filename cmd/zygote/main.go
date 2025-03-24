package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/evgnomon/zygote/internal/cert"
	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/db"
	"github.com/evgnomon/zygote/internal/mem"
	"github.com/evgnomon/zygote/internal/migration"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/utils"
)

const containerStartTimeout = 20 * time.Second
const httpClientTimeout = 10 * time.Second
const editor = "vi"

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
	logger, err := util.Logger()
	if err != nil {
		panic(err)
	}

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
					return m.Up(ctx, logger)
				},
			},
			{
				Name:  "down",
				Usage: "Revert the last applied migration to undo changes to the database schema.",
				Action: func(c *cli.Context) error {
					ctx := context.Background()
					m := NewMigration(c.String("directory"))
					return m.Down(ctx, logger)
				},
			},
		},
	}
}

func initCommand() *cli.Command {
	logger, err := util.Logger()
	if err != nil {
		panic(err)
	}
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
			err := initContainers(ctx, logger, c.String("directory"))
			return err
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
	logger, err := util.Logger()
	if err != nil {
		panic(err)
	}
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
			err := initContainers(context.Background(), logger, c.String("directory"))
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

					return cs.Sign([]string{c.String("name")}, time.Now().AddDate(1, 0, 0))
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
		},
		Action: func(c *cli.Context) error {
			u := c.String("url")
			client, err := Call()
			if err != nil {
				return err
			}
			r, err := client.R().Get(u)
			if err != nil {
				return err
			}
			fmt.Print(r.String())
			return nil
		},
	}
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
		Action: func(_ *cli.Context) error {
			os.Setenv("MYSQL_PWD", "root1234")
			err := utils.Run("mysql", "-u", "root", "-h", "127.0.0.1", "-s", "--auto-rehash")
			if err != nil {
				return err
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
			{
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
			},
			{
				Name:  "table",
				Usage: "Create a table",
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
			},
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
			openDiffs(),
			openActions(),
			sqlCommand(),
			generateCommand(),
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

func initContainers(ctx context.Context, logger *zap.Logger, directory string) error {
	numShards := 2
	dbName, err := utils.RepoFullName()
	if err != nil {
		return fmt.Errorf("failed to get repo full name: %w", err)
	}
	for i := 1; i <= numShards; i++ {
		sqlParams := container.SQLInitParams{
			DBName:   fmt.Sprintf("%s_%d", dbName, i),
			Username: "admin",
			Password: "password",
		}
		sqlStatements, err := container.SQLInit(sqlParams)
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
	return m.Up(ctx, logger)
}

func Call() (*resty.Client, error) {
	cs, err := cert.Cert()
	if err != nil {
		log.Fatalf("Failed to create cert service: %v", err)
	}

	clientName := "brave"
	clientCert, err := tls.LoadX509KeyPair(cs.FunctionCertFile(clientName), cs.FunctionKeyFile(clientName))
	if err != nil {
		return nil, err
	}

	serverCACert, err := os.ReadFile(cs.CaCertFile()) // The CA that signed the server's certificate
	if err != nil {
		return nil, err
	}

	serverCAs := x509.NewCertPool()
	if ok := serverCAs.AppendCertsFromPEM(serverCACert); !ok {
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
