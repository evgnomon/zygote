package controller

import (
	"context"
	"fmt"
	"net/http"
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

func (rc *RedisQueryController) AddEndpoint(prefix string, e *echo.Echo) error {
	e.Pre(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			defer rc.Close()
			return next(c)
		}
	})
	e.POST(fmt.Sprintf("%s/queries/mem", prefix), rc.QueryHandler)
	return nil
}
