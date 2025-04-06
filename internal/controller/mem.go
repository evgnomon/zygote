package controller

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/redis/go-redis/v9"
)

const redisTimeout = 5 * time.Second

type RedisQueryRequest struct {
	Query []string `json:"query" form:"query"`
}

type RedisQueryController struct {
	config    *RedisConfig
	client    *redis.ClusterClient
	mu        sync.Mutex
	lastCheck time.Time
}

type RedisConfig struct {
	Addrs    []string
	Password string
}

func NewRedisQueryController(config *RedisConfig) (*RedisQueryController, error) {
	if config == nil {
		config = &RedisConfig{
			// The rest of nodes are discovered by the client
			Addrs: []string{"shard-a.zygote.run:6373", "shard-b.zygote.run:6373", "shard-c.zygote.run:6373"},
		}
	}

	rc := &RedisQueryController{
		config: config,
	}

	if err := rc.ensureConnection(); err != nil {
		return nil, err
	}

	return rc, nil
}

func (rc *RedisQueryController) Close() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.client != nil {
		err := rc.client.Close()
		rc.client = nil
		return err
	}
	return nil
}

func (rc *RedisQueryController) ensureConnection() error {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if rc.client != nil && time.Since(rc.lastCheck) < 5*time.Minute {
		return nil
	}

	if rc.client != nil {
		rc.client.Close()
		rc.client = nil
	}

	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:    rc.config.Addrs,
		Password: rc.config.Password,
	})

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		client.Close()
		return err
	}

	rc.client = client
	rc.lastCheck = time.Now()
	return nil
}

func (rc *RedisQueryController) QueryHandler(c echo.Context) error {
	if err := rc.ensureConnection(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis connection failed: " + err.Error(),
		})
	}

	var req RedisQueryRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid request format",
		})
	}

	// Prepare command arguments for Redis
	args := make([]any, len(req.Query))
	for i, part := range req.Query {
		args[i] = part
	}

	// Execute Redis command with retry
	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	var result any
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		result, err = rc.client.Do(ctx, args...).Result()
		if err != nil {
			if strings.Contains(err.Error(), "connection") {
				if reconnErr := rc.ensureConnection(); reconnErr != nil {
					return c.JSON(http.StatusServiceUnavailable, map[string]string{
						"error": "Failed to reconnect to Redis: " + reconnErr.Error(),
					})
				}
				continue
			}
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": fmt.Sprintf("Command execution failed: %v", err),
			})
		}
		break
	}

	if err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Failed to execute command after multiple attempts",
		})
	}

	// Format response based on result type
	var response any
	switch v := result.(type) {
	case string:
		response = map[string]string{"result": v}
	case int64:
		response = map[string]int64{"result": v}
	case []any:
		strSlice := make([]string, len(v))
		for i, item := range v {
			if str, ok := item.(string); ok {
				strSlice[i] = str
			} else {
				strSlice[i] = fmt.Sprintf("%v", item)
			}
		}
		response = map[string][]string{"result": strSlice}
	default:
		response = map[string]any{"result": v}
	}

	return c.JSON(http.StatusOK, response)
}

// Add this new method to RedisQueryController
func (rc *RedisQueryController) ClusterNodesHandler(c echo.Context) error {
	if err := rc.ensureConnection(); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{
			"error": "Redis connection failed: " + err.Error(),
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisTimeout)
	defer cancel()

	// Get cluster nodes information
	nodes, err := rc.client.ClusterNodes(ctx).Result()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "Failed to get cluster nodes: " + err.Error(),
		})
	}

	// Parse the nodes string into a more structured JSON response
	type NodeInfo struct {
		ID          string   `json:"id"`
		Address     string   `json:"address"`
		Flags       []string `json:"flags"`
		Role        string   `json:"role"`
		MasterID    string   `json:"masterId,omitempty"`
		PingSent    int64    `json:"pingSent"`
		PongRecv    int64    `json:"pongRecv"`
		ConfigEpoch int64    `json:"configEpoch"`
		LinkState   string   `json:"linkState"`
		Slots       []string `json:"slots,omitempty"`
	}

	var clusterNodes []NodeInfo
	lines := strings.Split(nodes, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 8 { //nolint:mnd
			continue
		}

		flags := strings.Split(parts[2], ",")
		role := "master"
		if strings.Contains(parts[2], "slave") {
			role = "slave"
		}

		node := NodeInfo{
			ID:          parts[0],
			Address:     parts[1],
			Flags:       flags,
			Role:        role,
			MasterID:    parts[3],
			PingSent:    parseInt(parts[4]),
			PongRecv:    parseInt(parts[5]),
			ConfigEpoch: parseInt(parts[6]),
			LinkState:   parts[7],
		}

		// Add slots if present (for master nodes)
		if len(parts) > 8 { //nolint:mnd
			node.Slots = parts[8:]
		}

		clusterNodes = append(clusterNodes, node)
	}

	return c.JSON(http.StatusOK, map[string]any{
		"nodes": clusterNodes,
		"count": len(clusterNodes),
	})
}

// Helper function to parse integers safely
func parseInt(s string) int64 {
	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// Modify the AddEndpoint method to include the new endpoint
func (rc *RedisQueryController) AddEndpoint(prefix string, e *echo.Echo) error {
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer rc.Close()
			return next(c)
		}
	})
	e.POST(fmt.Sprintf("%s/mem/query", prefix), rc.QueryHandler)
	e.GET(fmt.Sprintf("%s/mem/cluster/node", prefix), rc.ClusterNodesHandler) // New endpoint
	return nil
}
