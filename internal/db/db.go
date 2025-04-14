package db

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/utils"
)

//go:embed templates/*.sql
var templates embed.FS

const mysqlImage = "evgnomon/mysql:8.4.4"
const plainFilePermission = 0644
const routerPortNumRead = 6446
const routerPortNumWrite = 6447
const sqlsDir = "sqls"
const mysqlPublicDefaultPort = 3306
const groupRepDefaultPort = 33061
const defaultShardSize = 3
const containerStartTimeout = 20 * time.Second
const hostNetworkName = "host"
const defaultConnDatabaseName = "mysql"
const localhostIP = "127.0.0.1"
const dbShortName = "sql"
const dbRouterShortName = "sql-router"
const replicationRetrySkeepTime = 5 * time.Second

var logger = util.NewLogger()

type SQLNode struct {
	Tenant       string
	Domain       string
	DatabaseName string
	User         string
	Password     string
	RootPassword string
	MigrationDir string
	NetworkName  string
	GroupName    string
	NumShards    int
	ShardSize    int
	ShardIndex   int
	RepIndex     int
}

func NewSQLNode() (*SQLNode, error) {
	c := &SQLNode{}
	if c.ShardSize == 0 {
		c.ShardSize = defaultShardSize
	}
	if c.DatabaseName == "" {
		dbName, err := utils.RepoFullName()
		if err != nil {
			return nil, fmt.Errorf("failed to get repo full name: %w", err)
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
	if c.RootPassword == "" {
		c.RootPassword = "root1234"
	}
	if c.Tenant == "" {
		c.Tenant = "zygote"
	}
	if c.NetworkName == "" {
		c.NetworkName = container.AppNetworkName()
	}
	if c.GroupName == "" {
		c.GroupName = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	}
	if c.Domain == "" {
		c.Domain = "zygote.run"
	}
	return c, nil
}

func mapPort(base int, repIndex, shardIndex int) int {
	return base + repIndex*10 + 100*shardIndex
}

func (r *SQLNode) mapPort(base int) int {
	if r.NetworkName == hostNetworkName {
		return base
	}
	return mapPort(base, r.RepIndex, r.ShardIndex)
}

func (c *SQLNode) MakeDB(ctx context.Context) error {
	dbName := c.DatabaseName
	var err error
	if dbName == "" {
		dbName, err = utils.RepoFullName()
		if err != nil {
			return fmt.Errorf("failed to get repo full name: %w", err)
		}
	}
	containerName := c.DBContainerName()
	containerConfig := &container.ContainerConfig{
		Name:        containerName,
		NetworkName: c.NetworkName,
		MysqlImage:  mysqlImage,
		HealthCommand: []string{
			"CMD",
			"mysql",
			"-h",
			"localhost",
			"-u",
			"root",
			fmt.Sprintf("-p%s", c.RootPassword),
			"-e",
			"SHOW tables;",
			dbName,
		},
		Bindings: []string{
			fmt.Sprintf("%s-data:/var/lib/mysql", containerName),
			fmt.Sprintf("%s-conf-gr:/etc/mysql/conf.d", containerName),
			fmt.Sprintf("%s-conf:/docker-entrypoint-initdb.d", containerName),
		},
		Caps: []string{"SYS_NICE"},
		EnvVars: []string{
			fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", c.RootPassword),
		},
		Ports: map[int]int{
			c.mapPort(mysqlPublicDefaultPort): mysqlPublicDefaultPort,
			c.mapPort(groupRepDefaultPort):    groupRepDefaultPort,
		},
	}
	return containerConfig.Make(ctx)
}

func (c *SQLNode) Endpoints() []string {
	if c.ShardSize == 0 {
		return nil
	}
	addrs := make([]string, c.ShardSize)
	if c.NetworkName != hostNetworkName {
		for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
			addrs[repIndex] = fmt.Sprintf("%s:%d", container.MapContainerName(dbShortName, c.Tenant, repIndex, c.ShardIndex), mysqlPublicDefaultPort)
		}
		return addrs
	}
	for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
		addrs[repIndex] = fmt.Sprintf("shard-%s.%s:%d", string('a'+rune(repIndex)), c.Domain, mysqlPublicDefaultPort)
		if c.ShardIndex > 0 {
			addrs[repIndex] = fmt.Sprintf("shard-%s-%d.%s:%d", string('a'+rune(repIndex)), c.ShardIndex, c.Domain, mysqlPublicDefaultPort)
		}
	}
	return addrs
}

func (c *SQLNode) GroupReplicationAddresses() []string {
	if c.ShardSize == 0 {
		return nil
	}
	addrs := make([]string, c.ShardSize)
	if c.NetworkName != hostNetworkName {
		for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
			addrs[repIndex] = fmt.Sprintf("%s:%d", container.MapContainerName(dbShortName, c.Tenant, repIndex, c.ShardIndex), groupRepDefaultPort)
		}
		return addrs
	}
	for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
		addrs[repIndex] = fmt.Sprintf("shard-%s.%s:%d", string('a'+rune(repIndex)), c.Domain, groupRepDefaultPort)
		if c.ShardIndex > 0 {
			addrs[repIndex] = fmt.Sprintf("shard-%s-%d.%s:%d", string('a'+rune(repIndex)), c.ShardIndex, c.Domain, groupRepDefaultPort)
		}
	}
	return addrs
}

func (c *SQLNode) GroupReplicationHosts(shardIndex int) []string {
	if c.ShardSize == 0 {
		return nil
	}
	addrs := make([]string, c.ShardSize)
	if c.NetworkName != hostNetworkName {
		for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
			addrs[repIndex] = container.MapContainerName(dbShortName, c.Tenant, repIndex, c.ShardIndex)
		}
		return addrs
	}
	for repIndex := 0; repIndex < c.ShardSize; repIndex++ {
		addrs[repIndex] = fmt.Sprintf("shard-%s.%s", string('a'+rune(repIndex)), c.Domain)
		if shardIndex > 0 {
			addrs[repIndex] = fmt.Sprintf("shard-%s-%d.%s", string('a'+rune(repIndex)), shardIndex, c.Domain)
		}
	}
	return addrs
}

func (c *SQLNode) Make(ctx context.Context) error {
	logger.Info("Creating SQL node...")
	
	err := c.MakeSQLReplica(ctx)
	if err != nil {
		return fmt.Errorf("failed to create SQL replica: %w", err)
	}

	retries := 5
	for retries > 0 {
		retries--
		err = c.JoinGroupReplication()
		if err != nil {
			logger.Info("Waiting for group replication to be ready...")
			time.Sleep(replicationRetrySkeepTime)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("failed to join group replication: %w", err)
	}

	return c.MakeSQLRouter(ctx)
}

func (c *SQLNode) ContainerName(name string) string {
	return container.MapContainerName(name, c.Tenant, c.RepIndex, c.ShardIndex)
}

func (c *SQLNode) MakeSQLRouter(ctx context.Context) error {
	const mysqlRouterConfTmplName = "router.conf"
	routerConfParams := container.RouterConfParams{
		Destinations: strings.Join(c.Endpoints(), ","),
	}
	routerConf, err := container.ApplyTemplate(mysqlRouterConfTmplName, routerConfParams)
	if err != nil {
		return err
	}
	containerName := c.ContainerName(dbRouterShortName)
	container.Vol(c.Tenant, routerConf, fmt.Sprintf("%s-conf", containerName),
		"/etc/mysqlrouter/", "router.conf", container.AppNetworkName())
	config := &container.ContainerConfig{
		Name:          c.ContainerName(dbRouterShortName),
		NetworkName:   c.NetworkName,
		MysqlImage:    mysqlImage,
		HealthCommand: []string{"CMD", "true"},
		Bindings: []string{
			fmt.Sprintf("%s:/etc/mysqlrouter/", fmt.Sprintf("%s-conf", containerName)),
		},
		Caps: []string{"SYS_NICE"},
		EnvVars: []string{
			fmt.Sprintf("MYSQL_PWD=%s", c.Password),
		},
		Cmd: []string{
			"mysqlrouter",
			"--config=/etc/mysqlrouter/router.conf",
		},
		Ports: map[int]int{
			c.mapPort(routerPortNumRead):  routerPortNumRead,
			c.mapPort(routerPortNumWrite): routerPortNumWrite,
		},
	}
	err = config.Make(ctx)
	if err != nil {
		return fmt.Errorf("failed to create router container: %w", err)
	}
	return nil
}

func (c *SQLNode) reportSQLInstanceAddress() string {
	if c.NetworkName != hostNetworkName {
		return c.ContainerName(dbShortName)
	}
	if c.ShardIndex == 0 {
		return fmt.Sprintf("shard-%s.%s", string('a'+rune(c.RepIndex)), c.Domain)
	}
	return fmt.Sprintf("shard-%s-%d.%s", string('a'+rune(c.RepIndex)), c.ShardIndex, c.Domain)
}

func (c *SQLNode) MakeGroupReplicationConfigVolume() error {
	const clusterTmplName = "innodb_cluster_template.cnf"
	sqlParams := container.InnoDBClusterParams{
		ServerID:             c.RepIndex + 1,
		GroupReplicationPort: groupRepDefaultPort,
		ServerCount:          c.ShardSize,
		ServersList:          strings.Join(c.GroupReplicationAddresses(), ","),
		ReportAddress:        c.reportSQLInstanceAddress(),
		ReportPort:           mysqlPublicDefaultPort,
	}
	innodbGroupReplication, err := container.ApplyTemplate(clusterTmplName, sqlParams)
	if err != nil {
		return err
	}
	container.Vol(c.Tenant, innodbGroupReplication, fmt.Sprintf("%s-conf-gr", c.ContainerName(dbShortName)),
		"/etc/mysql/conf.d/", "gr.cnf", container.AppNetworkName())
	return err
}

func (c *SQLNode) MakeSQLConfBVolume() error {
	const basicInitSQLTmplName = "sql_init_template.sql"
	basicInitParams := container.SQLInitParams{
		DBName:   c.DatabaseName,
		Username: c.User,
		Password: c.Password,
	}
	sqlStatements, err := container.ApplyTemplate(basicInitSQLTmplName, basicInitParams)
	if err != nil {
		return err
	}
	containerName := c.ContainerName("sql")
	container.Vol(c.Tenant, sqlStatements, fmt.Sprintf("%s-conf", containerName),
		"/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())
	return nil
}

func (c *SQLNode) MakeSQLReplica(ctx context.Context) error {
	err := c.MakeSQLConfBVolume()
	if err != nil {
		return fmt.Errorf("failed to create SQL config volume: %w", err)
	}
	err = c.MakeGroupReplicationConfigVolume()
	if err != nil {
		return fmt.Errorf("failed to create group replication config volume: %w", err)
	}
	return c.MakeDB(ctx)
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

func (c *SQLNode) DBContainerName() string {
	return c.ContainerName(dbShortName)
}

func (c *SQLNode) SQLRouterContainerName() string {
	return c.ContainerName(dbRouterShortName)
}

func (c *SQLNode) connectionString(dbName string) string {
	if c.NetworkName != hostNetworkName {
		return fmt.Sprintf("root:%s@tcp(%s:%d)/%s", c.RootPassword, localhostIP, c.mapPort(mysqlPublicDefaultPort), dbName)
	}
	return fmt.Sprintf("root:%s@tcp(%s:%d)/%s", c.RootPassword, localhostIP, mysqlPublicDefaultPort, dbName)
}

func (c *SQLNode) JoinGroupReplication() error {
	// Database connection parameters (adjust as needed)
	var db *sql.DB
	var err error
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	dsn := c.connectionString(defaultConnDatabaseName)
	// Connect to specific node
	db, err = sql.Open(defaultConnDatabaseName, dsn)
	if err != nil {
		return fmt.Errorf("error connecting to database index %d: %v", c.RepIndex, err)
	}

	// Common setup queries for all nodes
	queries := []string{
		"INSTALL PLUGIN group_replication SONAME 'group_replication.so'",
		fmt.Sprintf("SET GLOBAL group_replication_group_name = '%s'", c.GroupName),
		fmt.Sprintf("SET GLOBAL group_replication_local_address = '%s:%d'",
			c.GroupReplicationHosts(c.ShardIndex)[c.RepIndex], groupRepDefaultPort),
		fmt.Sprintf("SET GLOBAL group_replication_group_seeds = '%s'",
			strings.Join(c.GroupReplicationAddresses(), ",")),
		fmt.Sprintf("SET GLOBAL group_replication_ip_allowlist = '172.18.0.0/16,127.0.0.1,%s'",
			strings.Join(c.GroupReplicationHosts(c.ShardIndex), ",")),
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
			logger.Debug("Error executing query", util.M{
				"query":        query,
				"error":        err,
				"replicaIndex": c.RepIndex,
			})
			return fmt.Errorf("error executing query on replica index %d: %v", c.RepIndex, err)
		}
	}

	// Node-specific configuration
	if c.RepIndex == 0 {
		// Bootstrap node
		_, err = db.Exec("SET GLOBAL group_replication_bootstrap_group = ON")
		if err != nil {
			return fmt.Errorf("error setting bootstrap on replica index %d: %v", c.RepIndex, err)
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
				logger.Debug("Error executing secondary query", util.M{
					"query":        query,
					"error":        err,
					"replicaIndex": c.RepIndex,
				})
				return fmt.Errorf("error executing secondary query on replica index %d: %v", c.RepIndex, err)
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
			logger.Debug("Error executing final query", util.M{
				"query":        query,
				"error":        err,
				"replicaIndex": c.RepIndex,
			})
			return fmt.Errorf("error executing final query on replica index %d: %v", c.RepIndex, err)
		}
	}

	// Check replication status
	rows, err := db.Query("SELECT * FROM performance_schema.replication_group_members")
	if err != nil {
		return fmt.Errorf("error querying replication status on replica index %d: %v", c.RepIndex, err)
	}
	defer rows.Close()

	// Print replication group members
	for rows.Next() {
		var channelName, memberID, memberHost, memberRole, memberState, memVersion, memCom string
		var memberPort int
		err := rows.Scan(&channelName, &memberID, &memberHost,
			&memberPort, &memberRole, &memberState, &memVersion, &memCom)
		if err != nil {
			return fmt.Errorf("error scanning replication status on replica index %d: %v", c.RepIndex, err)
		}
		logger.Info("Replica added", util.M{
			"memberHost":  memberHost,
			"memberPort":  memberPort,
			"memberRole":  memberRole,
			"memberState": memberState,
		})
	}

	return nil
}
