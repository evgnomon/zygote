package mem

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/pkg/utils"
)

const defaultShardSize = 3
const hostNetworkName = "host"
const redisImage = "redis:7.4.2"
const defaultRedisPort = 6373

var logger = utils.NewLogger()
var defaultRedisPortStr = strconv.Itoa(defaultRedisPort)

type MemNode struct {
	Tenant       string
	Domain       string
	NetworkName  string
	ShardSize    int
	NumShards    int
	ShardIndex   int
	ReplicaIndex int
}

func NewMemNode() *MemNode {
	m := MemNode{}
	m.NetworkName = hostNetworkName
	m.ShardSize = defaultShardSize
	return &m
}

func (m *MemNode) CreateReplica(ctx context.Context) {
	config := container.ContainerConfig{
		Name:        utils.NodeContainer("mem", m.Tenant, m.ReplicaIndex, m.ShardIndex),
		NetworkName: m.NetworkName,
		Image:       redisImage, // Assuming redisImage is the image used
		HealthCommand: []string{
			"CMD",
			"redis-cli",
			"-p",
			defaultRedisPortStr,
			"ping",
			"--raw",
			"incr",
			"ping",
		},
		Bindings: []string{
			fmt.Sprintf("%s-data:/var/lib/redis", utils.NodeContainer("mem", m.Tenant, m.ReplicaIndex, m.ShardIndex)),
		},
		Caps:    []string{},
		EnvVars: []string{},
		Cmd: []string{
			"redis-server",
			"--port",
			defaultRedisPortStr,
			"--cluster-enabled",
			"yes",
			"--cluster-node-timeout",
			"5000",
			"--appendonly",
			"yes",
		},
		Ports: map[int]int{
			defaultRedisPort + m.ReplicaIndex*10 + m.ShardIndex*100: defaultRedisPort,
		},
	}
	err := config.StartContainer(ctx)
	if err != nil {
		logger.Fatal("Failed to start Redis container", utils.M{"error": err})
		return
	}
}

func (m *MemNode) createRedisClusterCommand() []string {
	// Base command parts
	cmd := []string{"redis-cli", "--cluster", "create"}
	// Generate shard hostnames based on shardSize
	for repIndex := 0; repIndex < m.ShardSize; repIndex++ {
		for shardIndex := 0; shardIndex < m.NumShards; shardIndex++ {
			// Convert shard number to letter (a=0, b=1, c=2, etc.)
			shardLetter := string('a' + rune(repIndex))
			var host string
			if shardIndex%m.NumShards == 0 {
				var host string
				if m.NetworkName != hostNetworkName {
					host = fmt.Sprintf("%s:%d", utils.NodeContainer("mem", m.Tenant, repIndex, shardIndex), defaultRedisPort)
				} else {
					host = fmt.Sprintf("shard-%s.%s:%d", shardLetter, m.Domain, defaultRedisPort)
				}
				cmd = append(cmd, host)
				continue
			}
			if m.NetworkName != hostNetworkName {
				host = fmt.Sprintf("%s:%d", utils.NodeContainer("mem", m.Tenant, repIndex, shardIndex), defaultRedisPort)
			} else {
				host = fmt.Sprintf("shard-%s-%d.%s:%d", shardLetter, shardIndex%m.NumShards, m.Domain, defaultRedisPort)
			}
			cmd = append(cmd, host)
		}
	}

	// Add replica and confirmation flags
	cmd = append(cmd, "--cluster-replicas", fmt.Sprintf("%d", m.ShardSize-1), "--cluster-yes")
	return cmd
}

func (m *MemNode) Init(ctx context.Context) {
	if m.ReplicaIndex != 0 || m.ShardIndex != 0 {
		return
	}
	logger.Debug("Creating Redis cluster", utils.M{"replicaIndex": m.ReplicaIndex, "shardIndex": m.ShardIndex})
	portMap := map[string]string{
		defaultRedisPortStr: defaultRedisPortStr,
	}
	client, err := container.CreateClinet()

	logger.FatalIfErr("Create client for container", err)

	command := strings.Join(m.createRedisClusterCommand(), " ")
	logger.Debug("Redis command", utils.M{"command": command})

	// Configure backoff parameters
	backoffConfig := utils.BackoffConfig{
		MaxAttempts:  10,
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
	}

	// Use exponential backoff for container creation
	err = backoffConfig.Retry(ctx, func() error {
		return container.SpawnAndWait(ctx, client, redisImage, m.Tenant, m.createRedisClusterCommand(), portMap, m.NetworkName)
	})
	logger.FatalIfErr("Create redis cluster", err)
}
