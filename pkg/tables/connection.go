package tables

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/evgnomon/zygote/pkg/http"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/go-sql-driver/mysql"
)

const defaultReplica = 0

var logger = utils.NewLogger()

// ClientConfig holds database connection configuration
type ClientConfig struct {
	User            string
	Password        string
	Host            string
	ReadPort        int
	WritePort       int
	Database        string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// SQLConnection defines the interface for database connection
type SQLConnection interface {
	ConnectRead(shardIndex int) (*sql.DB, error)
	ConnectWrite(shardIndex int) (*sql.DB, error)
	ConnectAllShardsRead() (map[int]*sql.DB, error)
	ConnectAllShardsWrite() (map[int]*sql.DB, error)
	Close(shardIndex int) error
	CloseAll() error
	GetReadConnection(shardIndex int) (*sql.DB, error)
	GetWriteConnection(shardIndex int) (*sql.DB, error)
	RetryReadOperation(shardIndex int, operation func(*sql.DB) error) error
	RetryWriteOperation(shardIndex int, operation func(*sql.DB) error) error
}

// ShardEndpoint represents a shard's connection details
type ShardEndpoint struct {
	Index     int
	Host      string
	ReadPort  int
	WritePort int
}

// MultiDBConnector manages multiple database connections
type MultiDBConnector struct {
	network         string
	configs         map[int]*ClientConfig
	readConns       map[int]*sql.DB
	writeConns      map[int]*sql.DB
	mutex           sync.RWMutex
	domain          string
	taregtReadPort  int
	targetWritePort int
	tenant          string
	databsae        string
	numShards       int
}

// NewMultiDBConnector creates a new multi-connection manager
func NewMultiDBConnector(network, tenant, baseHost, database string, targetReadPort, targetWritePort, numShards int) *MultiDBConnector {
	return &MultiDBConnector{
		configs:         make(map[int]*ClientConfig),
		readConns:       make(map[int]*sql.DB),
		writeConns:      make(map[int]*sql.DB),
		domain:          baseHost,
		taregtReadPort:  targetReadPort,
		targetWritePort: targetWritePort,
		network:         network,
		tenant:          tenant,
		databsae:        database,
		numShards:       numShards,
	}
}

// NewClientConfig creates a default configuration
func NewClientConfig(targetReadPort, targetWritePort int) *ClientConfig {
	return &ClientConfig{
		User:            "root",
		Password:        "root1234",
		Host:            "127.0.0.1",
		ReadPort:        targetReadPort,
		WritePort:       targetWritePort,
		Database:        "mysql",
		MaxOpenConns:    25,
		MaxIdleConns:    25,
		ConnMaxLifetime: 5 * time.Minute,
	}
}

// CalculateShardEndpoints generates shard endpoints
func CalculateShardEndpoints(network, domain string, numShards,
	targetReadPort, targetWritePort int) ([]ShardEndpoint, error) {
	if numShards <= 0 {
		return nil, fmt.Errorf("shard count must be positive")
	}
	if targetReadPort <= 0 || targetWritePort <= 0 {
		return nil, fmt.Errorf("base ports must be positive")
	}

	endpoints := make([]ShardEndpoint, numShards)
	for shardIndex := 0; shardIndex < numShards; shardIndex++ {
		endpoints[shardIndex] = ShardEndpoint{
			Index:     shardIndex,
			Host:      utils.RemoteHost(network, domain, defaultReplica, shardIndex),
			ReadPort:  utils.NodePort(network, targetReadPort, 0, shardIndex),
			WritePort: utils.NodePort(network, targetWritePort, 0, shardIndex),
		}
	}
	return endpoints, nil
}

// AddConfig adds a new database configuration
func (m *MultiDBConnector) AddConfig(shardIndex int, config *ClientConfig) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.configs[shardIndex] = config
	return nil
}

type connectionType string

const (
	readConn  connectionType = "read"
	writeConn connectionType = "write"
)

// connect establishes a database connection for a shard
func (m *MultiDBConnector) connect(ctx context.Context, shardIndex int, connType connectionType) (*sql.DB, error) {
	var db *sql.DB
	var err error
	b := utils.BackoffConfig{
		MaxAttempts:  3,
		InitialDelay: 5 * time.Second,
		MaxDelay:     1 * time.Minute,
	}
	err = b.Retry(ctx, func() error {
		// Get config
		m.mutex.RLock()
		config, exists := m.configs[shardIndex]
		m.mutex.RUnlock()

		if !exists {
			return fmt.Errorf("configuration for shard index %d not found", shardIndex)
		}

		// Select appropriate connection map and port
		var conns map[int]*sql.DB
		var port int
		switch connType {
		case readConn:
			conns = m.readConns
			port = config.ReadPort
		case writeConn:
			conns = m.writeConns
			port = config.WritePort
		default:
			return fmt.Errorf("invalid connection type: %s", connType)
		}

		m.mutex.Lock()
		defer m.mutex.Unlock()

		var ok bool
		// Check existing connection
		if db, ok = conns[shardIndex]; ok && db.Ping() == nil {
			return nil
		}

		// Construct DSN
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true",
			config.User,
			config.Password,
			config.Host,
			port,
			config.Database,
		)

		// Create connection
		db, err = sql.Open("mysql", dsn)
		if err != nil {
			return fmt.Errorf("failed to connect to shard %d %s: %v", shardIndex, connType, err)
		}

		// Configure connection pool
		db.SetMaxOpenConns(config.MaxOpenConns)
		db.SetMaxIdleConns(config.MaxIdleConns)
		db.SetConnMaxLifetime(config.ConnMaxLifetime)

		// Test connection
		if err := db.Ping(); err != nil {
			db.Close()
			return fmt.Errorf("failed to ping shard %d %s: %v", shardIndex, connType, err)
		}

		// Store connection
		conns[shardIndex] = db
		return nil
	})
	return db, err
}

// ConnectRead establishes a read connection for a shard
func (m *MultiDBConnector) ConnectRead(ctx context.Context, shardIndex int) (*sql.DB, error) {
	return m.connect(ctx, shardIndex, readConn)
}

// ConnectWrite establishes a write connection for a shard
func (m *MultiDBConnector) ConnectWrite(ctx context.Context, shardIndex int) (*sql.DB, error) {
	return m.connect(ctx, shardIndex, writeConn)
}

// ConnectAllShardsRead connects to all shards for read in parallel
func (m *MultiDBConnector) ConnectAllShardsRead(ctx context.Context) (map[int]*sql.DB, error) {
	endpoints, err := CalculateShardEndpoints(m.network, m.domain, m.numShards, m.taregtReadPort, m.targetWritePort)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shard endpoints: %v", err)
	}

	readDBs := make(map[int]*sql.DB)
	var connectErrors []error
	var mu sync.Mutex
	var wg sync.WaitGroup
	logger.Debug("Connecting to shards for read", utils.M{"endpoints": endpoints})

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Create config for shard
			config := NewClientConfig(endpoint.ReadPort, endpoint.WritePort)
			config.Host = endpoint.Host
			config.ReadPort = endpoint.ReadPort
			config.WritePort = endpoint.WritePort
			config.Database = m.databsae

			// Add config
			if err := m.AddConfig(index, config); err != nil {
				mu.Lock()
				connectErrors = append(connectErrors, fmt.Errorf("failed to add config for shard %d: %v", index, err))
				mu.Unlock()
				return
			}

			// Connect to shard read
			db, err := m.ConnectRead(ctx, index)
			if err != nil {
				mu.Lock()
				connectErrors = append(connectErrors, fmt.Errorf("failed to connect to shard %d read: %v", index, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			readDBs[index] = db
			mu.Unlock()
		}(endpoint.Index)
	}

	wg.Wait()

	if len(connectErrors) > 0 {
		return readDBs, fmt.Errorf("encountered errors connecting to shards: %v", connectErrors)
	}

	return readDBs, nil
}

// ConnectAllShardsWrite connects to all shards for write in parallel
func (m *MultiDBConnector) ConnectAllShardsWrite(ctx context.Context) (map[int]*sql.DB, error) {
	endpoints, err := CalculateShardEndpoints(m.network, m.domain, m.numShards, m.taregtReadPort, m.targetWritePort)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate shard endpoints: %v", err)
	}

	writeDBs := make(map[int]*sql.DB)
	var connectErrors []error
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Create config for shard
			config := NewClientConfig(endpoint.ReadPort, endpoint.WritePort)
			config.Host = endpoint.Host
			config.ReadPort = endpoint.ReadPort
			config.WritePort = endpoint.WritePort

			// Add config
			if err := m.AddConfig(index, config); err != nil {
				mu.Lock()
				connectErrors = append(connectErrors, fmt.Errorf("failed to add config for shard %d: %v", index, err))
				mu.Unlock()
				return
			}

			// Connect to shard write
			db, err := m.ConnectWrite(ctx, index)
			if err != nil {
				mu.Lock()
				connectErrors = append(connectErrors, fmt.Errorf("failed to connect to shard %d write: %v", index, err))
				mu.Unlock()
				return
			}

			mu.Lock()
			writeDBs[index] = db
			mu.Unlock()
		}(endpoint.Index)
	}

	wg.Wait()

	if len(connectErrors) > 0 {
		return writeDBs, fmt.Errorf("encountered errors connecting to shards: %v", connectErrors)
	}

	return writeDBs, nil
}

// GetReadConnection retrieves an existing read connection
func (m *MultiDBConnector) GetReadConnection(shardIndex int) (*sql.DB, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	db, exists := m.readConns[shardIndex]
	if !exists {
		return nil, fmt.Errorf("no read connection found for shard %d", shardIndex)
	}

	return db, nil
}

// GetWriteConnection retrieves an existing write connection
func (m *MultiDBConnector) GetWriteConnection(shardIndex int) (*sql.DB, error) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	db, exists := m.writeConns[shardIndex]
	if !exists {
		return nil, fmt.Errorf("no write connection found for shard %d", shardIndex)
	}

	return db, nil
}

// Close closes both read and write connections for a shard
func (m *MultiDBConnector) Close(shardIndex int) error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	logger.Debug("Closing connections", utils.M{"shardIndex": shardIndex})

	var errs []error

	if readDB, exists := m.readConns[shardIndex]; exists {
		if err := readDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close read connection for shard %d: %v", shardIndex, err))
		}
		delete(m.readConns, shardIndex)
	}

	if writeDB, exists := m.writeConns[shardIndex]; exists {
		if err := writeDB.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close write connection for shard %d: %v", shardIndex, err))
		}
		delete(m.writeConns, shardIndex)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections for shard %d: %v", shardIndex, errs)
	}

	return nil
}

// CloseAll closes all read and write connections
func (m *MultiDBConnector) CloseAll() error {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	logger.Debug("Closing all connections")

	var errs []error

	for shardIndex, db := range m.readConns {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close read connection for shard %d: %v", shardIndex, err))
		}
		delete(m.readConns, shardIndex)
	}

	for shardIndex, db := range m.writeConns {
		if err := db.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close write connection for shard %d: %v", shardIndex, err))
		}
		delete(m.writeConns, shardIndex)
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}

// isTransientError checks if an error is worth retrying
func isTransientError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, sql.ErrConnDone) || errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if mysqlErr, ok := err.(*mysql.MySQLError); ok {
		return mysqlErr.Number == 2006 || mysqlErr.Number == 2013
	}
	return strings.Contains(strings.ToLower(err.Error()), "connection refused") ||
		strings.Contains(strings.ToLower(err.Error()), "bad connection") ||
		strings.Contains(strings.ToLower(err.Error()), "network")
}

// RetryOperation executes a database operation (read or write) with retries and backoff
func (m *MultiDBConnector) RetryOperation(ctx context.Context, shardIndex int, operation func(*sql.DB) error, isWrite bool) error {
	var db *sql.DB
	var err error

	if isWrite {
		db, err = m.GetWriteConnection(shardIndex)
	} else {
		db, err = m.GetReadConnection(shardIndex)
	}
	if err != nil {
		opType := "read"
		if isWrite {
			opType = "write"
		}
		return fmt.Errorf("failed to get %s connection for shard %d: %v", opType, shardIndex, err)
	}

	ic := utils.BackoffConfig{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     20 * time.Second,
	}

	return ic.Retry(ctx, func() error {
		err = operation(db)
		if err != nil {
			if isTransientError(err) {
				return err
			}
			opType := "read"
			if isWrite {
				opType = "write"
			}
			return backoff.Permanent(fmt.Errorf("%s operation failed for shard %d: %v", opType, shardIndex, err))
		}
		return nil
	})
}

// RetryReadOperation executes a read operation with retries and backoff
func (m *MultiDBConnector) RetryReadOperation(ctx context.Context, shardIndex int, operation func(*sql.DB) error) error {
	return m.RetryOperation(ctx, shardIndex, operation, false)
}

// RetryWriteOperation executes a write operation with retries and backoff
func (m *MultiDBConnector) RetryWriteOperation(ctx context.Context, shardIndex int, operation func(*sql.DB) error) error {
	return m.RetryOperation(ctx, shardIndex, operation, true)
}

// GenericQueryHandler handles SQL queries with a provided query and struct type
func (m *MultiDBConnector) GenericQueryHandler(ctx context.Context, shardIndex int, query string, resultStruct any, c http.Context) error {
	return m.RetryReadOperation(ctx, shardIndex, func(db *sql.DB) error {
		rows, err := db.QueryContext(c.GetRequestContext(), query)
		if err != nil {
			return c.SendInternalError("Failed to execute query: ", err)
		}
		defer rows.Close()

		// Get the slice type for results
		sliceType := reflect.TypeOf(resultStruct)
		if sliceType.Kind() != reflect.Slice {
			return c.SendInternalError("Result struct must be a slice", err)
		}

		// Create a slice to hold results
		results := reflect.New(sliceType).Elem()
		elemType := sliceType.Elem()

		// Iterate over rows
		for rows.Next() {
			// Create a new instance of the struct
			elem := reflect.New(elemType).Elem()

			// Get fields to scan
			fields := make([]any, elem.NumField())
			for i := 0; i < elem.NumField(); i++ {
				fields[i] = elem.Field(i).Addr().Interface()
			}

			// Scan row into struct fields
			if err := rows.Scan(fields...); err != nil {
				return c.SendInternalError("Failed to scan results", err)
			}

			// Append to results slice
			results = reflect.Append(results, elem)
		}

		// Check for errors during iteration
		if err = rows.Err(); err != nil {
			return c.SendInternalError("Error reading results", err)
		}

		return c.Send(map[string]any{
			"results": results.Interface(),
		})
	})
}
