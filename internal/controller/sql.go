package controller

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/tables"
	"github.com/labstack/echo/v4"
)

const routerReadPort = 6446
const routerWritePort = 6447
const defaultNumShards = 3

var logger = util.NewLogger()

type SQLQueryRequest struct {
	Query string `json:"query" form:"query"`
}

type SQLQueryController struct {
	connector *tables.MultiDBConnector
}

func NewSQLQueryController() (*SQLQueryController, error) {
	// Initialize database configuration
	ctx := context.Background()
	connector := tables.NewMultiDBConnector(container.AppNetworkName(), "zygote", "my.zygote.run", "mysql",
		routerReadPort, routerWritePort, defaultNumShards)
	_, err := connector.ConnectAllShardsRead(ctx)
	if err != nil {
		return nil, err
	}
	_, err = connector.ConnectAllShardsWrite(ctx)
	if err != nil {
		return nil, err
	}
	dc := &SQLQueryController{
		connector: connector,
	}
	return dc, nil
}

// Close cleans up database resources
func (dc *SQLQueryController) Close() error {
	logger.Debug("Closing database connections")
	return dc.connector.CloseAll()
}

// QueryHandler handles SQL query requests
func (dc *SQLQueryController) QueryHandler(c echo.Context) error {
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
	return dc.connector.RetryReadOperation(c.Request().Context(), 0, func(db *sql.DB) error {
		rows, err := db.QueryContext(c.Request().Context(), query)
		if err != nil {
			return err
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
	})
}

// ClusterMember defines the structure for cluster member info
type ClusterMember struct {
	MemberID      string `json:"member_id"`
	MemberHost    string `json:"member_host"`
	MemberPort    int    `json:"member_port"`
	MemberState   string `json:"member_state"`
	MemberRole    string `json:"member_role"`
	MemberVersion string `json:"member_version"`
}

// ClusterStatusHandler is a specific handler using GenericQueryHandler
func (dc *SQLQueryController) ClusterStatusHandler(c echo.Context) error {
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
	var results = []ClusterMember{}
	err := dc.connector.GenericQueryHandler(c.Request().Context(), 0, query, results, c)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to execute query: " + err.Error(),
		})
	}
	return nil
}

// AddEndpoint configures the controller routes
func (dc *SQLQueryController) AddEndpoint(prefix string, e *echo.Echo) error {
	e.POST(fmt.Sprintf("%s/sql/query", prefix), dc.QueryHandler)
	e.GET(fmt.Sprintf("%s/sql/cluster", prefix), dc.ClusterStatusHandler)
	return nil
}
