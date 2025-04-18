package mem

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/internal/util"
	"github.com/evgnomon/zygote/pkg/utils"
)

const defaultShardSize = 3
const hostNetworkName = "host"
const redisImage = "redis:7.4.2"
const defaultRedisPort = 6373

var logger = util.NewLogger()
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
		logger.Fatal("Failed to start Redis container", util.M{"error": err})
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

func (m *MemNode) Init(ctx context.Context) error {
	portMap := map[string]string{
		defaultRedisPortStr: defaultRedisPortStr,
	}
	client, err := container.CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	logger.Debug("Redis command", util.M{"command": strings.Join(m.createRedisClusterCommand(), " ")})
	return container.SpawnAndWait(ctx, client, redisImage, m.Tenant, m.createRedisClusterCommand(), portMap, m.NetworkName)
}
