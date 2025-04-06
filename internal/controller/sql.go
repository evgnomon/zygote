package controller

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/evgnomon/zygote/pkg/tables"
	"github.com/labstack/echo/v4"
)

type SQLQueryRequest struct {
	Query string `json:"query" form:"query"`
}

type SQLQueryController struct {
	config    *tables.ClientConfig
	db        *sql.DB
	mu        sync.Mutex // For thread-safe DB reconnection
	lastCheck time.Time
}

func NewSQLQueryController() (*SQLQueryController, error) {
	// Initialize database configuration
	config := tables.NewClientConfig()

	// You might want to get this from environment variables or config
	if customDB := ""; customDB != "" { // Replace with actual config source
		config.Database = customDB
	}

	dc := &SQLQueryController{
		config: tables.NewClientConfig(),
	}

	if err := dc.ensureConnection(); err != nil {
		return nil, err
	}

	return dc, nil
}

// Close cleans up database resources
func (dc *SQLQueryController) Close() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	if dc.db != nil {
		dc.db.Close()
		dc.db = nil
	}
	return nil
}

// ensureConnection checks and maintains database connection
func (dc *SQLQueryController) ensureConnection() error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// If we have a connection and it's been checked recently, return
	if dc.db != nil && time.Since(dc.lastCheck) < 5*time.Minute {
		return nil
	}

	// Close existing connection if it exists
	if dc.db != nil {
		dc.db.Close()
		dc.db = nil
	}

	// Establish new connection
	var connector tables.DatabaseConnector = dc.config
	db, err := connector.Connect()
	if err != nil {
		return err
	}

	// Verify connection
	if err = db.Ping(); err != nil {
		db.Close()
		return err
	}

	dc.db = db
	dc.lastCheck = time.Now()
	return nil
}

// QueryHandler handles SQL query requests
func (dc *SQLQueryController) QueryHandler(c echo.Context) error {
	// Ensure we have a working connection
	if err := dc.ensureConnection(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Database connection failed: " + err.Error(),
		})
	}

	// Parse request
	var req SQLQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	query := strings.TrimSpace(req.Query)
	if query == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Query cannot be empty",
		})
	}

	// Execute query with retry on connection loss
	var rows *sql.Rows
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		rows, err = dc.db.Query(query)
		if err != nil {
			// If it's a connection error, try to reconnect
			if err == sql.ErrConnDone || strings.Contains(err.Error(), "connection") {
				if reconnErr := dc.ensureConnection(); reconnErr != nil {
					return c.JSON(http.StatusServiceUnavailable, map[string]string{
						"error": "Failed to reconnect to database: " + reconnErr.Error(),
					})
				}
				continue
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Query execution failed: " + err.Error(),
			})
		}
		break
	}

	if rows == nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Failed to execute query after multiple attempts",
		})
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get columns: " + err.Error(),
		})
	}

	// Process results
	var results []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		valuePtrs := make([]any, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to scan row: " + err.Error(),
			})
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
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Row iteration error: " + err.Error(),
		})
	}

	return c.JSON(http.StatusOK, results)
}

// ClusterStatusHandler handles requests for InnoDB cluster status
func (dc *SQLQueryController) ClusterStatusHandler(c echo.Context) error {
	// Ensure DB connection is valid
	dc.mu.Lock()
	if err := dc.db.Ping(); err != nil {
		dc.mu.Unlock()
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Database connection failed: " + err.Error(),
		})
	}
	dc.mu.Unlock()

	// Query InnoDB cluster status
	query := `
        SELECT 
            MEMBER_ID,
            MEMBER_HOST,
            MEMBER_PORT,
            MEMBER_STATE,
            MEMBER_ROLE,
            MEMBER_VERSION
        FROM 
            performance_schema.replication_group_members
    `

	rows, err := dc.db.QueryContext(c.Request().Context(), query)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to query cluster status: " + err.Error(),
		})
	}
	defer rows.Close()

	// Define structure for cluster member info
	type ClusterMember struct {
		MemberID      string `json:"member_id"`
		MemberHost    string `json:"member_host"`
		MemberPort    int    `json:"member_port"`
		MemberState   string `json:"member_state"`
		MemberRole    string `json:"member_role"`
		MemberVersion string `json:"member_version"`
	}

	var members []ClusterMember

	// Scan results
	for rows.Next() {
		var member ClusterMember
		err := rows.Scan(
			&member.MemberID,
			&member.MemberHost,
			&member.MemberPort,
			&member.MemberState,
			&member.MemberRole,
			&member.MemberVersion,
		)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "Failed to scan cluster status: " + err.Error(),
			})
		}
		members = append(members, member)
	}

	// Check for errors during iteration
	if err = rows.Err(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Error reading cluster status: " + err.Error(),
		})
	}

	// Return successful response
	return c.JSON(http.StatusOK, map[string]any{
		"members": members,
	})
}

// AddEndpoint configures the controller routes
func (dc *SQLQueryController) AddEndpoint(prefix string, e *echo.Echo) error {
	// Cleanup on server shutdown
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer dc.Close()
			return next(c)
		}
	})
	e.POST(fmt.Sprintf("%s/sql/query", prefix), dc.QueryHandler)
	e.GET(fmt.Sprintf("%s/sql/cluster", prefix), dc.ClusterStatusHandler)
	return nil
}
