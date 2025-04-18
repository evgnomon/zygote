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
const hostNetworkName = "host"
const defaultConnDatabaseName = "mysql"
const localhostIP = "127.0.0.1"
const dbShortName = "sql"
const dbRouterShortName = "sql-router"
const replicationRetrySleepTime = 5 * time.Second
const numRetriesJoinGroupReplication = 5

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

func NewSQLNode() *SQLNode {
	c := &SQLNode{}
	if c.ShardSize == 0 {
		c.ShardSize = defaultShardSize
	}
	if c.DatabaseName == "" {
		dbName := utils.RepoFullName()
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
	return c
}

func (s *SQLNode) mapPort(target int) int {
	return utils.NodePort(s.NetworkName, target, s.RepIndex, s.ShardIndex)
}

func (s *SQLNode) MakeDB(ctx context.Context) error {
	dbName := s.DatabaseName
	if dbName == "" {
		dbName = utils.RepoFullName()
	}
	containerName := s.DBContainerName()
	containerConfig := &container.ContainerConfig{
		Name:        containerName,
		NetworkName: s.NetworkName,
		Image:       mysqlImage,
		HealthCommand: []string{
			"CMD",
			"mysql",
			"-h",
			"localhost",
			"-u",
			"root",
			fmt.Sprintf("-p%s", s.RootPassword),
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
			fmt.Sprintf("MYSQL_ROOT_PASSWORD=%s", s.RootPassword),
		},
		Ports: map[int]int{
			s.mapPort(mysqlPublicDefaultPort): mysqlPublicDefaultPort,
			s.mapPort(groupRepDefaultPort):    groupRepDefaultPort,
		},
	}
	return containerConfig.StartContainer(ctx)
}

func (s *SQLNode) generateAddresses(port int) []string {
	if s.ShardSize == 0 {
		return nil
	}

	addrs := make([]string, s.ShardSize)
	if s.NetworkName != hostNetworkName {
		for repIndex := 0; repIndex < s.ShardSize; repIndex++ {
			addrs[repIndex] = fmt.Sprintf("%s:%d",
				utils.NodeContainer(dbShortName, s.Tenant, repIndex, s.ShardIndex),
				port)
		}
		return addrs
	}

	for repIndex := 0; repIndex < s.ShardSize; repIndex++ {
		shardName := string('a' + rune(repIndex))
		if s.ShardIndex > 0 {
			addrs[repIndex] = fmt.Sprintf("shard-%s-%d.%s:%d",
				shardName, s.ShardIndex, s.Domain, port)
		} else {
			addrs[repIndex] = fmt.Sprintf("shard-%s.%s:%d",
				shardName, s.Domain, port)
		}
	}
	return addrs
}

func (s *SQLNode) Endpoints() []string {
	return s.generateAddresses(mysqlPublicDefaultPort)
}

func (s *SQLNode) GroupReplicationAddresses() []string {
	return s.generateAddresses(groupRepDefaultPort)
}

func (s *SQLNode) GroupReplicationHosts(shardIndex int) []string {
	if s.ShardSize == 0 {
		return nil
	}
	addrs := make([]string, s.ShardSize)
	if s.NetworkName != hostNetworkName {
		for repIndex := 0; repIndex < s.ShardSize; repIndex++ {
			addrs[repIndex] = utils.NodeContainer(dbShortName, s.Tenant, repIndex, s.ShardIndex)
		}
		return addrs
	}
	for repIndex := 0; repIndex < s.ShardSize; repIndex++ {
		addrs[repIndex] = fmt.Sprintf("shard-%s.%s", string('a'+rune(repIndex)), s.Domain)
		if shardIndex > 0 {
			addrs[repIndex] = fmt.Sprintf("shard-%s-%d.%s", string('a'+rune(repIndex)), shardIndex, s.Domain)
		}
	}
	return addrs
}

func (s *SQLNode) StartSQLContainers(ctx context.Context) error {
	logger.Debug("Start SQL node containers", util.M{"sqlNode": s})
	err := s.MakeSQLReplica(ctx)
	if err != nil {
		return fmt.Errorf("failed to create SQL replica: %w", err)
	}
	err = s.MakeSQLRouter(ctx)
	logger.FatalIfErr("failed to create SQL router", err)

	logger.Debug("SQL node containers started", util.M{"sqlNode": s})
	retries := numRetriesJoinGroupReplication
	for retries > 0 {
		retries--
		err = s.JoinGroupReplication()
		if err != nil {
			logger.Debug("Waiting for group replication to be ready...")
			time.Sleep(replicationRetrySleepTime)
			continue
		}
		break
	}
	if err != nil {
		return fmt.Errorf("failed to join group replication: %w", err)
	}
	return nil
}

func (s *SQLNode) ContainerName(name string) string {
	return utils.NodeContainer(name, s.Tenant, s.RepIndex, s.ShardIndex)
}

func (s *SQLNode) MakeSQLRouter(ctx context.Context) error {
	const mysqlRouterConfTmplName = "router.conf"
	routerConfParams := container.RouterConfParams{
		Destinations: strings.Join(s.Endpoints(), ","),
	}
	routerConf, err := container.ApplyTemplate(mysqlRouterConfTmplName, routerConfParams)
	if err != nil {
		return err
	}
	containerName := s.ContainerName(dbRouterShortName)
	container.Vol(s.Tenant, routerConf, fmt.Sprintf("%s-conf", containerName),
		"/etc/mysqlrouter/", "router.conf", container.AppNetworkName())
	config := &container.ContainerConfig{
		Name:          s.ContainerName(dbRouterShortName),
		NetworkName:   s.NetworkName,
		Image:         mysqlImage,
		HealthCommand: []string{"CMD", "true"},
		Bindings: []string{
			fmt.Sprintf("%s:/etc/mysqlrouter/", fmt.Sprintf("%s-conf", containerName)),
		},
		Caps: []string{"SYS_NICE"},
		EnvVars: []string{
			fmt.Sprintf("MYSQL_PWD=%s", s.Password),
		},
		Cmd: []string{
			"mysqlrouter",
			"--config=/etc/mysqlrouter/router.conf",
		},
		Ports: map[int]int{
			s.mapPort(routerPortNumRead):  routerPortNumRead,
			s.mapPort(routerPortNumWrite): routerPortNumWrite,
		},
	}
	err = config.StartContainer(ctx)
	if err != nil {
		return fmt.Errorf("failed to create router container: %w", err)
	}
	return nil
}

func (s *SQLNode) reportSQLInstanceAddress() string {
	if s.NetworkName != hostNetworkName {
		return s.ContainerName(dbShortName)
	}
	if s.ShardIndex == 0 {
		return fmt.Sprintf("shard-%s.%s", string('a'+rune(s.RepIndex)), s.Domain)
	}
	return fmt.Sprintf("shard-%s-%d.%s", string('a'+rune(s.RepIndex)), s.ShardIndex, s.Domain)
}

func (s *SQLNode) MakeGroupReplicationConfigVolume() error {
	const clusterTmplName = "innodb_cluster_template.cnf"
	sqlParams := container.InnoDBClusterParams{
		ServerID:             s.RepIndex + 1,
		GroupReplicationPort: groupRepDefaultPort,
		ServerCount:          s.ShardSize,
		ServersList:          strings.Join(s.GroupReplicationAddresses(), ","),
		ReportAddress:        s.reportSQLInstanceAddress(),
		ReportPort:           mysqlPublicDefaultPort,
	}
	innodbGroupReplication, err := container.ApplyTemplate(clusterTmplName, sqlParams)
	if err != nil {
		return err
	}
	container.Vol(s.Tenant, innodbGroupReplication, fmt.Sprintf("%s-conf-gr", s.ContainerName(dbShortName)),
		"/etc/mysql/conf.d/", "gr.cnf", container.AppNetworkName())
	return err
}

func (s *SQLNode) MakeSQLConfBVolume() error {
	const basicInitSQLTmplName = "sql_init_template.sql"
	basicInitParams := container.SQLInitParams{
		DBName:   s.DatabaseName,
		Username: s.User,
		Password: s.Password,
	}
	sqlStatements, err := container.ApplyTemplate(basicInitSQLTmplName, basicInitParams)
	if err != nil {
		return err
	}
	containerName := s.ContainerName("sql")
	container.Vol(s.Tenant, sqlStatements, fmt.Sprintf("%s-conf", containerName),
		"/docker-entrypoint-initdb.d", "init.sql", container.AppNetworkName())
	return nil
}

func (s *SQLNode) MakeSQLReplica(ctx context.Context) error {
	err := s.MakeSQLConfBVolume()
	if err != nil {
		return fmt.Errorf("failed to create SQL config volume: %w", err)
	}
	err = s.MakeGroupReplicationConfigVolume()
	if err != nil {
		return fmt.Errorf("failed to create group replication config volume: %w", err)
	}
	return s.MakeDB(ctx)
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

func (s *SQLNode) DBContainerName() string {
	return s.ContainerName(dbShortName)
}

func (s *SQLNode) SQLRouterContainerName() string {
	return s.ContainerName(dbRouterShortName)
}

func (s *SQLNode) connectionString(dbName string) string {
	if s.NetworkName != hostNetworkName {
		return fmt.Sprintf("root:%s@tcp(%s:%d)/%s", s.RootPassword, localhostIP, s.mapPort(mysqlPublicDefaultPort), dbName)
	}
	return fmt.Sprintf("root:%s@tcp(%s:%d)/%s", s.RootPassword, localhostIP, mysqlPublicDefaultPort, dbName)
}

func (s *SQLNode) JoinGroupReplication() error {
	logger.Debug("Joining group replication", util.M{"replicaIndex": s.RepIndex,
		"shardIndex": s.ShardIndex, "domain": s.Domain, "tenant": s.Tenant})
	var db *sql.DB
	var err error
	defer func() {
		if db != nil {
			db.Close()
		}
	}()
	dsn := s.connectionString(defaultConnDatabaseName)
	// Connect to specific node
	db, err = sql.Open(defaultConnDatabaseName, dsn)
	if err != nil {
		logger.Debug("Error connecting to database", util.M{
			"dsn":          dsn,
			"replicaIndex": s.RepIndex,
			"shardIndex":   s.ShardIndex,
		})
		return fmt.Errorf("error connecting to database repindex %d shardIndex %d: %v", s.RepIndex, s.ShardIndex, err)
	}

	// Common setup queries for all nodes
	queries := []string{
		"INSTALL PLUGIN group_replication SONAME 'group_replication.so'",
		fmt.Sprintf("SET GLOBAL group_replication_group_name = '%s'", s.GroupName),
		fmt.Sprintf("SET GLOBAL group_replication_local_address = '%s:%d'",
			s.GroupReplicationHosts(s.ShardIndex)[s.RepIndex], groupRepDefaultPort),
		fmt.Sprintf("SET GLOBAL group_replication_group_seeds = '%s'",
			strings.Join(s.GroupReplicationAddresses(), ",")),
		fmt.Sprintf("SET GLOBAL group_replication_ip_allowlist = '172.18.0.0/16,127.0.0.1,%s'",
			strings.Join(s.GroupReplicationHosts(s.ShardIndex), ",")),
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
				"replicaIndex": s.RepIndex,
				"shardIndex":   s.ShardIndex,
			})
			return fmt.Errorf("error executing query on replica index %d, shardIndex: %d, %w", s.RepIndex, s.ShardIndex, err)
		}
	}

	// Node-specific configuration
	if s.RepIndex == 0 {
		// Bootstrap node
		_, err = db.Exec("SET GLOBAL group_replication_bootstrap_group = ON")
		if err != nil {
			return fmt.Errorf("error setting bootstrap on replica index %d: %v", s.RepIndex, err)
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
					"replicaIndex": s.RepIndex,
				})
				return fmt.Errorf("error executing secondary query on replica index %d: %v", s.RepIndex, err)
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
				"replicaIndex": s.RepIndex,
			})
			return fmt.Errorf("error executing final query on replica index %d: %v", s.RepIndex, err)
		}
	}

	// Check replication status
	rows, err := db.Query("SELECT * FROM performance_schema.replication_group_members")
	if err != nil {
		return fmt.Errorf("error querying replication status on replica index %d: %v", s.RepIndex, err)
	}
	defer rows.Close()

	// Print replication group members
	for rows.Next() {
		var channelName, memberID, memberHost, memberRole, memberState, memVersion, memCom string
		var memberPort int
		err := rows.Scan(&channelName, &memberID, &memberHost,
			&memberPort, &memberRole, &memberState, &memVersion, &memCom)
		if err != nil {
			return fmt.Errorf("error scanning replication status on replica index %d: %v", s.RepIndex, err)
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
