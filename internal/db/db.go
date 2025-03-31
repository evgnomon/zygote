package db

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"log"
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

const mysqlImage = "evgnomon/mysql:8.4.4"
const plainFilePermission = 0644
const sqlsDir = "sqls"
const mysqlPublicPort = 3306
const groupRepPort = 33061
const defaultShardSize = 3
const containerStartTimeout = 20 * time.Second
const clusterTmplName = "innodb_cluster_template.cnf"
const basicInitSQLTmplName = "sql_init_template.sql"
const mysqlRouterConfTmplName = "router.conf"

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
			Cmd: []string{"mysqld", "--mysql-native-password=ON"},
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
			RestartPolicy: dcontainer.RestartPolicy{
				Name: dcontainer.RestartPolicyAlways,
			},
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

func CreateGroupReplicationContainer(numReplicas int, networkName string) {
	ctx := context.Background()
	for i := 1; i <= numReplicas; i++ {
		var r Replica
		r.Index = i-1
		r.NetworkName = networkName
		r.AdminPasswrod = "password"
		r.RootPasswrod = "root1234"
		r.Tenant = "zygote"
		r.Create(ctx)
	}
}

type Replica struct {
	Index         int
	NetworkName   string
	RootPasswrod  string
	AdminPasswrod string
	Tenant        string
}

func (r *Replica) Create(ctx context.Context) {
	cli, err := container.CreateClinet()
	if err != nil {
		panic(err)
	}
	envVars := []string{
		// "MYSQL_ROOT_PASSWORD=root1234",
		fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", r.RootPasswrod),
	}
	dbName, err := utils.RepoFullName()
	if err != nil {
		panic(err)
	}
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
				fmt.Sprintf("-p%s", r.AdminPasswrod),
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
					HostPort: fmt.Sprintf("%d", 3306+r.Index),
				},
			},
		},
		Binds: []string{
			fmt.Sprintf("%s-db-%d-data:/var/lib/mysql", r.Tenant, r.Index+1),
			fmt.Sprintf("%s-db-conf-gr-%d:/etc/mysql/conf.d", r.Tenant, r.Index+1),
			fmt.Sprintf("%s-db-conf-%d:/docker-entrypoint-initdb.d", r.Tenant, r.Index+1),
		},
		CapAdd: []string{"SYS_NICE"},
		RestartPolicy: dcontainer.RestartPolicy{
			Name: dcontainer.RestartPolicyAlways,
		},
	}
	_, err = cli.NetworkInspect(ctx, r.NetworkName, networktypes.InspectOptions{})
	if err != nil {
		_, err = cli.NetworkCreate(ctx, r.NetworkName, networktypes.CreateOptions{})
	}
	if err != nil {
		panic(err)
	}
	if r.NetworkName != "" {
		hostConfig.NetworkMode = dcontainer.NetworkMode(r.NetworkName)
	}
	container.Pull(ctx, mysqlImage)
	containerName := fmt.Sprintf("%s-db-rep-%d", r.Tenant, r.Index+1)
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
	container.WaitHealthy(r.Tenant+"-", containerStartTimeout)
}

func CreateRouter(networkName string) {
	CreateContainer(
		1,
		networkName,
		"zygote-db-router",
		mysqlImage,
		[]string{"CMD", "true"},
		[]string{
			"zygote-db-router-conf-%d:/etc/mysqlrouter/",
		},
		[]string{"SYS_NICE"}, []string{
			"MYSQL_PWD=root1234",
		},
		[]string{"mysqlrouter", "--config=/etc/mysqlrouter/router.conf"},
		map[int]int{6446: 16446, 6447: 17447}, //nolint: gomnd
	)
}

type Cluster struct {
	Tenant       string
	Domain       string
	DatabaseName string
	User         string
	Password     string
	MigrationDir string
	NetworkName  string
	GroupName    string
	NumShards    int
	Size         int
}

func (c *Cluster) PublicAddresses(shardIndex int) []string {
	if c.Size == 0 {
		return nil
	}
	addrs := make([]string, c.Size)
	for i := 1; i <= c.Size; i++ {
		addrs[i] = fmt.Sprintf("shard-%s.%s:%d", string('a'+rune(i)), c.Domain, mysqlPublicPort)
		if shardIndex > 0 {
			addrs[i] = fmt.Sprintf("shard-%s-%d.%s:%d", string('a'+rune(i)), shardIndex, c.Domain, mysqlPublicPort)
		}
	}
	return addrs
}

func (c *Cluster) GroupReplicationAddresses(shardIndex int) []string {
	if c.Size == 0 {
		return nil
	}
	addrs := make([]string, c.Size)
	for i := 1; i <= c.Size; i++ {
		addrs[i] = fmt.Sprintf("shard-%s.%s:%d", string('a'+rune(i)), c.Domain, groupRepPort)
		if shardIndex > 0 {
			addrs[i] = fmt.Sprintf("shard-%s-%d.%s:%d", string('a'+rune(i)), shardIndex, c.Domain, groupRepPort)
		}
	}
	return addrs
}

func (c *Cluster) DefaultValues() error {
	if c.Size == 0 {
		c.Size = defaultShardSize
	}
	if c.DatabaseName == "" {
		dbName, err := utils.RepoFullName()
		if err != nil {
			return fmt.Errorf("failed to get repo full name: %w", err)
		}
		c.DatabaseName = dbName
	}
	if c.NumShards == 0 {
		c.NumShards = 1
	}
	if c.User == "" {
		c.User = "admin"
	}
	if c.Password == "" {
		c.Password = "password"
	}
	if c.Tenant == "" {
		c.Tenant = "zygote"
	}
	if c.NetworkName == "" {
		c.NetworkName = container.AppNetworkName()
	}
	return nil
}

func (c *Cluster) Create(ctx context.Context, shardIndex int, repIndex int) error {
	err := c.DefaultValues()
	if err != nil {
		return err
	}
	c.CreateInstance(ctx, shardIndex, repIndex)
	c.SetAsGroupReplica(shardIndex, repIndex)
	c.CreateRouter()
	// mem.CreateMemContainer(3, container.AppNetworkName())
	// container.InitRedisCluster()
	// m := NewMigration(c.MigrationDir)
	// return m.Up(ctx)
	return nil
}

func (c *Cluster) CreateRouter() {
	CreateContainer(
		1,
		c.NetworkName,
		fmt.Sprintf("%s-db-router", c.Tenant),
		mysqlImage,
		[]string{"CMD", "true"},
		[]string{
			c.Tenant + "-db-router-conf-%d:/etc/mysqlrouter/",
		},
		[]string{"SYS_NICE"}, []string{
			fmt.Sprintf("MYSQL_PWD=%s", c.Password),
		},
		[]string{"mysqlrouter", "--config=/etc/mysqlrouter/router.conf"},
		map[int]int{6446: 16446, 6447: 17447}, //nolint: gomnd
	)
	container.WaitHealthy(c.Tenant+"-db-router-", containerStartTimeout)
}

func (c *Cluster) CreateInstance(ctx context.Context, shardIndex, repIndex int) error {
	err := c.DefaultValues()
	if err != nil {
		return err
	}
	sqlParams := container.InnoDBClusterParams{
		ServerID:             repIndex + 1,
		GroupReplicationPort: groupRepPort,
		ServerCount:          c.Size,
		ServersList:          strings.Join(c.GroupReplicationAddresses(shardIndex), ","),
	}
	innodbGroupReplication, err := container.ApplyTemplate(clusterTmplName, sqlParams)
	if err != nil {
		return err
	}
	basicInitParams := container.SQLInitParams{
		DBName:   c.DatabaseName,
		Username: c.User,
		Password: c.Password,
	}
	sqlStatements, err := container.ApplyTemplate(basicInitSQLTmplName, basicInitParams)
	if err != nil {
		return err
	}
	routerConfParams := container.RouterConfParams{
		Destinations: strings.Join(c.PublicAddresses(shardIndex), ","),
	}
	routerConf, err := container.ApplyTemplate(mysqlRouterConfTmplName, routerConfParams)
	if err != nil {
		return err
	}
	container.Vol(sqlStatements, fmt.Sprintf("%s-db-conf-%d", c.Tenant, repIndex),
		"/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())
	container.Vol(innodbGroupReplication, fmt.Sprintf("%s-db-conf-gr-%d", c.Tenant, repIndex),
		"/etc/mysql/conf.d/", "gr.cnf", container.AppNetworkName())
	container.Vol(routerConf, fmt.Sprintf("%s-db-router-conf-%d", c.Tenant, repIndex),
		"/etc/mysqlrouter/", "router.conf", container.AppNetworkName())
	var r Replica
	r.Index = repIndex
	r.NetworkName = container.AppNetworkName()
	r.AdminPasswrod = "password"
	r.RootPasswrod = "root1234"
	r.Tenant = "zygote"
	r.Create(ctx)
	return nil
}

func CreateContainer(numContainers int, networkName, prefix, mysqlImage string, healthCommand, bindings,
	caps, envVars, cmd []string, ports map[int]int) {
	ctx := context.Background()
	cli, err := container.CreateClinet()
	if err != nil {
		panic(err)
	}

	exposedPorts := nat.PortSet{}

	for target := range ports {
		exposedPorts[nat.Port(fmt.Sprint(target))] = struct{}{}
	}

	for i := 1; i <= numContainers; i++ {
		config := &dcontainer.Config{
			Image:        mysqlImage,
			Env:          envVars,
			ExposedPorts: exposedPorts,
			Healthcheck: &dcontainer.HealthConfig{
				Test:     healthCommand,
				Timeout:  20 * time.Second,
				Retries:  20,
				Interval: 1 * time.Second,
			},
			Cmd: cmd,
		}

		natBindings := map[nat.Port][]nat.PortBinding{}

		for target, exposed := range ports {
			natBindings[nat.Port(fmt.Sprint(target))] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: fmt.Sprintf("%d", exposed+i-1),
				},
			}
		}

		vBindings := []string{}

		for _, bind := range bindings {
			vBindings = append(vBindings, fmt.Sprintf(bind, i))
		}

		hostConfig := &dcontainer.HostConfig{
			PortBindings: natBindings,
			Binds:        vBindings,
			CapAdd:       caps,
			RestartPolicy: dcontainer.RestartPolicy{
				Name: dcontainer.RestartPolicyAlways,
			},
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
		containerName := fmt.Sprintf("%s-%d", prefix, i)
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
	current := time.Now().UTC()
	nano := current.UnixNano()
	prefix := fmt.Sprintf("%d_%s", nano, m.Desc)
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
		Desc: fmt.Sprintf("db_%s", dbName),
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
		Desc: fmt.Sprintf("table_%s_%s", dbName, tableName),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}

type CreateColumnParams struct {
	TableName    string
	DatabaseName string
	SQLColType   string
	ColumnName   string
	DefaultValue string
}

func resolveDefaultValue(sqlColType string) (string, error) {
	var defaultValue string
	switch sqlColType {
	case "INT", "BIGINT":
		defaultValue = "0"
	case "FLOAT", "DOUBLE":
		defaultValue = "0.0"
	case "BOOLEAN":
		defaultValue = "false"
	case "MEDIUMBLOB":
		defaultValue = "''"
	case "JSON":
		defaultValue = "NULL"
	case "VARCHAR(255)", "MEDIUMTEXT", "CHAR(36)":
		defaultValue = "''"
	default:
		return "", fmt.Errorf("unsupported column type: %s", sqlColType)
	}
	return defaultValue, nil
}

func CreateColumn(dbName, tableName, name, sqlColType string) (*SQLMigration, error) {
	upTemplate, err := templates.ReadFile("templates/create_column_up.sql")
	if err != nil {
		return nil, err
	}
	downTemplate, err := templates.ReadFile("templates/create_column_down.sql")
	if err != nil {
		return nil, err
	}
	tmplUp := string(upTemplate)
	tmplDown := string(downTemplate)
	tUp, err := template.New("create_column_up.sql").Parse(tmplUp)
	if err != nil {
		return nil, err
	}
	tDown, err := template.New("create_column_down.sql").Parse(tmplDown)
	if err != nil {
		return nil, err
	}

	defaultValue, err := resolveDefaultValue(sqlColType)
	if err != nil {
		return nil, err
	}

	var tplUp bytes.Buffer
	params := &CreateColumnParams{TableName: tableName, DatabaseName: dbName,
		ColumnName: name, SQLColType: sqlColType, DefaultValue: defaultValue}
	if err := tUp.Execute(&tplUp, params); err != nil {
		return nil, err
	}
	var tplDown bytes.Buffer
	if err := tDown.Execute(&tplDown, params); err != nil {
		return nil, err
	}
	result := &SQLMigration{
		Desc: fmt.Sprintf("column_%s_%s_%s", dbName, tableName, name),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}

type CreatePropertyParams struct {
	TableName    string
	DatabaseName string
	FieldPath    string
	ColumnName   string
	DataType     string
	Virtual      bool
}

func CreateProperty(dbName, tableName, name, fieldPath, dataType string, virtual bool) (*SQLMigration, error) {
	upTemplate, err := templates.ReadFile("templates/create_property_up.sql")
	if err != nil {
		return nil, err
	}
	downTemplate, err := templates.ReadFile("templates/create_property_down.sql")
	if err != nil {
		return nil, err
	}
	tmplUp := string(upTemplate)
	tmplDown := string(downTemplate)
	tUp, err := template.New("create_property_up.sql").Parse(tmplUp)
	if err != nil {
		return nil, err
	}
	tDown, err := template.New("create_property_down.sql").Parse(tmplDown)
	if err != nil {
		return nil, err
	}

	var tplUp bytes.Buffer
	params := &CreatePropertyParams{TableName: tableName, DatabaseName: dbName,
		ColumnName: name, FieldPath: fieldPath, DataType: dataType, Virtual: virtual}
	if err := tUp.Execute(&tplUp, params); err != nil {
		return nil, err
	}
	var tplDown bytes.Buffer
	if err := tDown.Execute(&tplDown, params); err != nil {
		return nil, err
	}
	result := &SQLMigration{
		Desc: fmt.Sprintf("prop_%s_%s_%s", dbName, tableName, name),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}

type CreateSQLParams struct {
	TableName    string
	DatabaseName string
	Type         string
	Name         string
}

func (p CreateSQLParams) GetTableName() string {
	return p.TableName
}

func (p CreateSQLParams) GetDatabaseName() string {
	return p.DatabaseName
}

func (p CreateSQLParams) GetType() string {
	return p.Type
}

func (p CreateSQLParams) GetName() string {
	return p.Name
}

type MigrationSpec interface {
	GetTableName() string
	GetDatabaseName() string
	GetType() string
	GetName() string
}

func GenCreateSQL(params MigrationSpec) (*SQLMigration, error) {
	upTemplate, err := templates.ReadFile(fmt.Sprintf("templates/create_%s_up.sql", params.GetType()))
	if err != nil {
		return nil, err
	}
	downTemplate, err := templates.ReadFile(fmt.Sprintf("templates/create_%s_down.sql", params.GetType()))
	if err != nil {
		return nil, err
	}
	tmplUp := string(upTemplate)
	tmplDown := string(downTemplate)
	tUp, err := template.New(fmt.Sprintf("create_%s_up.sql", params.GetType())).Parse(tmplUp)
	if err != nil {
		return nil, err
	}
	tDown, err := template.New(fmt.Sprintf("create_%s_down.sql", params.GetType())).Parse(tmplDown)
	if err != nil {
		return nil, err
	}

	var tplUp bytes.Buffer
	if err := tUp.Execute(&tplUp, params); err != nil {
		return nil, err
	}
	var tplDown bytes.Buffer
	if err := tDown.Execute(&tplDown, params); err != nil {
		return nil, err
	}
	result := &SQLMigration{
		Desc: fmt.Sprintf("%s_%s_%s_%s", params.GetType(), params.GetDatabaseName(),
			params.GetTableName(), params.GetName()),
		Up:   strings.Trim(tplUp.String(), "\n"),
		Down: strings.Trim(tplDown.String(), "\n"),
	}
	return result, nil
}

type CreateIndexParams struct {
	CreateSQLParams
	Columns  []string
	Unique   bool
	FullText bool
}

func SetupGroupReplication() {
	// Database connection parameters (adjust as needed)
	var db *sql.DB
	var err error
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	for i := 1; i <= 3; i++ {
		dsn := fmt.Sprintf("root:root1234@tcp(127.0.0.1:%d)/mysql", 3306+i-1)
		// Connect to specific node
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Printf("Error connecting to database %d: %v", i, err)
			continue
		}

		// Common setup queries for all nodes
		queries := []string{
			"INSTALL PLUGIN group_replication SONAME 'group_replication.so'",
			"SET GLOBAL group_replication_group_name = 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee'",
			fmt.Sprintf("SET GLOBAL group_replication_local_address = 'zygote-db-rep-%d:33061'", i),
			"SET GLOBAL group_replication_group_seeds = 'zygote-db-rep-1:33061,zygote-db-rep-2:33061,zygote-db-rep-3:33061'",
			"SET SQL_LOG_BIN = 0",
			"CREATE USER 'repl'@'%' IDENTIFIED with mysql_native_password BY 'strong_password'",
			"GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'repl'@'%'",
			"FLUSH PRIVILEGES",
			"SET SQL_LOG_BIN = 1",
		}

		// Execute common queries
		for _, query := range queries {
			_, err := db.Exec(query)
			if err != nil {
				log.Printf("Error executing query on node %d: %v - Query: %s", i, err, query)
			}
		}

		// Node-specific configuration
		if i == 1 {
			// Bootstrap node
			_, err = db.Exec("SET GLOBAL group_replication_bootstrap_group = ON")
			if err != nil {
				log.Printf("Error setting bootstrap on node %d: %v", i, err)
			}
		} else {
			// Secondary nodes
			secondaryQueries := []string{
				"STOP GROUP_REPLICATION",
				"RESET BINARY LOGS AND GTIDS",
				"RESET REPLICA ALL",
				"CHANGE REPLICATION SOURCE TO SOURCE_USER = 'repl', SOURCE_PASSWORD = 'strong_password' FOR CHANNEL 'group_replication_recovery'",
			}

			for _, query := range secondaryQueries {
				_, err := db.Exec(query)
				if err != nil {
					log.Printf("Error executing secondary query on node %d: %v - Query: %s", i, err, query)
				}
			}
		}

		// Start replication and cleanup
		finalQueries := []string{
			"START GROUP_REPLICATION",
			"SET GLOBAL group_replication_bootstrap_group = OFF",
		}

		for _, query := range finalQueries {
			_, err := db.Exec(query)
			if err != nil {
				log.Printf("Error executing final query on node %d: %v - Query: %s", i, err, query)
			}
		}

		// Check replication status
		rows, err := db.Query("SELECT * FROM performance_schema.replication_group_members")
		if err != nil {
			log.Printf("Error querying replication status on node %d: %v", i, err)
			continue
		}

		// Print replication group members
		fmt.Printf("Replication group members for node %d:\n", i)
		for rows.Next() {
			var channelName, memberID, memberHost, memberRole, memberState, memVersion, memCom string
			var memberPort int
			err := rows.Scan(&channelName, &memberID, &memberHost,
				&memberPort, &memberRole, &memberState, &memVersion, &memCom)
			if err != nil {
				log.Printf("Error scanning row on node %d: %v", i, err)
				continue
			}
			fmt.Printf("Member: %s, Host: %s:%d, Role: %s, State: %s\n",
				memberID, memberHost, memberPort, memberRole, memberState)
		}

		rows.Close()
		db.Close()
	}
}

func (c *Cluster) SetAsGroupReplica(shardIndex, repIndex int) error {
	// Database connection parameters (adjust as needed)
	var db *sql.DB
	var err error
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	dsn := fmt.Sprintf("root:root1234@tcp(127.0.0.1:%d)/mysql", 3306+repIndex)
	// Connect to specific node
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("Error connecting to database index %d: %v", repIndex, err)
	}

	// Common setup queries for all nodes
	queries := []string{
		"SET SQL_LOG_BIN = 0",
		"INSTALL PLUGIN group_replication SONAME 'group_replication.so'",
		"SET GLOBAL group_replication_group_name = '%s'",
		fmt.Sprintf("SET GLOBAL group_replication_local_address = '%s-db-rep-%d:%d'", c.DatabaseName, repIndex+1, groupRepPort),
		fmt.Sprintf("SET GLOBAL group_replication_group_seeds = '%s'", strings.Join(c.GroupReplicationAddresses(shardIndex), ",")),
		"SET SQL_LOG_BIN = 0",
		"CREATE USER 'repl'@'%' IDENTIFIED with mysql_native_password BY 'strong_password'",
		"GRANT REPLICATION SLAVE, REPLICATION CLIENT ON *.* TO 'repl'@'%'",
		"FLUSH PRIVILEGES",
		"SET SQL_LOG_BIN = 1",
	}

	// Execute common queries
	for _, query := range queries {
		_, err := db.Exec(query)
		if err != nil {
			log.Printf("Error executing query on replica index %d: %v - Query: %s", repIndex, err, query)
		}
	}

	// Node-specific configuration
	if repIndex == 0 {
		// Bootstrap node
		_, err = db.Exec("SET GLOBAL group_replication_bootstrap_group = ON")
		if err != nil {
			log.Printf("Error setting bootstrap on replica index %d: %v", repIndex, err)
		}
	} else {
		// Secondary nodes
		secondaryQueries := []string{
			"STOP GROUP_REPLICATION",
			"RESET BINARY LOGS AND GTIDS",
			"RESET REPLICA ALL",
			"CHANGE REPLICATION SOURCE TO SOURCE_USER = 'repl', SOURCE_PASSWORD = 'strong_password' FOR CHANNEL 'group_replication_recovery'",
		}

		for _, query := range secondaryQueries {
			_, err := db.Exec(query)
			if err != nil {
				log.Printf("Error executing secondary query on replica index %d: %v - Query: %s", repIndex, err, query)
			}
		}
	}

	// Start replication and cleanup
	finalQueries := []string{
		"START GROUP_REPLICATION",
		"SET GLOBAL group_replication_bootstrap_group = OFF",
	}

	for _, query := range finalQueries {
		_, err := db.Exec(query)
		if err != nil {
			log.Printf("Error executing final query on replica %d: %v - Query: %s", repIndex, err, query)
		}
	}

	// Check replication status
	rows, err := db.Query("SELECT * FROM performance_schema.replication_group_members")
	if err != nil {
		return fmt.Errorf("Error querying replication status on replica index %d: %v", repIndex, err)
	}
	defer rows.Close()

	// Print replication group members
	fmt.Printf("Replication group members for replica index %d:\n", repIndex)
	for rows.Next() {
		var channelName, memberID, memberHost, memberRole, memberState, memVersion, memCom string
		var memberPort int
		err := rows.Scan(&channelName, &memberID, &memberHost,
			&memberPort, &memberRole, &memberState, &memVersion, &memCom)
		if err != nil {
			log.Printf("Error scanning row on replica index %d: %v", repIndex, err)
			continue
		}
		fmt.Printf("Member: %s, Host: %s:%d, Role: %s, State: %s\n",
			memberID, memberHost, memberPort, memberRole, memberState)
	}

	return nil
}
