package db

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/pkg/utils"
)

//go:embed templates/*.sql
var templates embed.FS

const mysqlImage = "mysql:8.0.33"
const plainFilePermission = 0644
const sqlsDir = "sqls"

func CreateDBContainer(numShards int, networkName string) {
	ctx := context.Background()
	cli, err := container.CreateClinet()
	if err != nil {
		panic(err)
	}

	envVars := []string{
		"MYSQL_ROOT_PASSWORD=root1234",
	}

	dbName, err := utils.RepoFullName()
	if err != nil {
		panic(err)
	}

	for i := 1; i <= numShards; i++ {
		config := &dcontainer.Config{
			Image: mysqlImage,
			Env:   envVars,
			ExposedPorts: nat.PortSet{
				"3306": struct{}{},
			},
			Healthcheck: &dcontainer.HealthConfig{
				Test: []string{"CMD",
					"mysql",
					"-h",
					"localhost",
					"-u",
					"admin",
					"-ppassword",
					"-e",
					"SHOW tables;",
					dbName,
				},
				Timeout:  20 * time.Second,
				Retries:  20,
				Interval: 1 * time.Second,
			},
		}

		hostConfig := &dcontainer.HostConfig{
			PortBindings: nat.PortMap{
				"3306": []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: fmt.Sprintf("%d", 3306+i-1),
					},
				},
			},
			Binds: []string{
				fmt.Sprintf("zygote-db-%d-data:/var/lib/mysql", i),
				fmt.Sprintf("zygote-db-conf-%d:/docker-entrypoint-initdb.d", i),
			},
			CapAdd: []string{"SYS_NICE"},
		}

		_, err = cli.NetworkInspect(ctx, networkName, networktypes.InspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, networkName, networktypes.CreateOptions{})
		}
		if err != nil {
			panic(err)
		}

		if networkName != "" {
			hostConfig.NetworkMode = dcontainer.NetworkMode(networkName)
		}

		container.Pull(ctx, mysqlImage)
		containerName := fmt.Sprintf("zygote-db-shard-%d", i)
		resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
		if err != nil {
			if errdefs.IsConflict(err) {
				fmt.Printf("Container already exists: %s\n", containerName)
				return
			}
			panic(err)
		}

		if err := cli.ContainerStart(ctx, resp.ID, dcontainer.StartOptions{}); err != nil {
			panic(err)
		}
	}
}

type SQLMigration struct {
	Desc string
	Up   string
	Down string
}

func (m *SQLMigration) Save() error {
	if err := os.MkdirAll(sqlsDir, os.ModePerm); err != nil {
		return err
	}
	timestamp := time.Now().UTC().Format("20060102150405")
	prefix := fmt.Sprintf("%s_%s_", timestamp, m.Desc)
	upFileName := filepath.Join(sqlsDir, prefix+".up.sql")
	downFileName := filepath.Join(sqlsDir, prefix+".down.sql")

	if err := os.WriteFile(upFileName, []byte(m.Up), plainFilePermission); err != nil { // #nosec
		return err
	}
	if err := os.WriteFile(downFileName, []byte(m.Down), plainFilePermission); err != nil { // #nosec
		return err
	}

	return nil
}

func CreateDatabase(dbName string) (*SQLMigration, error) {
	upTemplate, err := templates.ReadFile("templates/create_db_up.sql")
	if err != nil {
		return nil, err
	}
	downTemplate, err := templates.ReadFile("templates/create_db_down.sql")
	if err != nil {
		return nil, err
	}
	tmplUp := string(upTemplate)
	tmplDown := string(downTemplate)
	tUp, err := template.New("create_db_up.sql").Parse(tmplUp)
	if err != nil {
		return nil, err
	}
	tDown, err := template.New("create_db_down.sql").Parse(tmplDown)
	if err != nil {
		return nil, err
	}
	var tplUp bytes.Buffer
	if err := tUp.Execute(&tplUp, struct{ DatabaseName string }{DatabaseName: dbName}); err != nil {
		return nil, err
	}
	var tplDown bytes.Buffer
	if err := tDown.Execute(&tplDown, struct{ DatabaseName string }{DatabaseName: dbName}); err != nil {
		return nil, err
	}
	result := &SQLMigration{
		Desc: fmt.Sprintf("create_db_%s", dbName),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}

type CreateTableParams struct {
	TableName    string
	DatabaseName string
}

func CreateTable(dbName, tableName string) (*SQLMigration, error) {
	upTemplate, err := templates.ReadFile("templates/create_table_up.sql")
	if err != nil {
		return nil, err
	}
	downTemplate, err := templates.ReadFile("templates/create_table_down.sql")
	if err != nil {
		return nil, err
	}
	tmplUp := string(upTemplate)
	tmplDown := string(downTemplate)
	tUp, err := template.New("create_table_up.sql").Parse(tmplUp)
	if err != nil {
		return nil, err
	}
	tDown, err := template.New("create_table_down.sql").Parse(tmplDown)
	if err != nil {
		return nil, err
	}
	var tplUp bytes.Buffer
	if err := tUp.Execute(&tplUp, &CreateTableParams{TableName: tableName, DatabaseName: dbName}); err != nil {
		return nil, err
	}
	var tplDown bytes.Buffer
	if err := tDown.Execute(&tplDown, &CreateTableParams{TableName: tableName, DatabaseName: dbName}); err != nil {
		return nil, err
	}
	result := &SQLMigration{
		Desc: fmt.Sprintf("create_table_%s_%s", dbName, tableName),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}
