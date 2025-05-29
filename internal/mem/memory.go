package mem

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/evgnomon/zygote/internal/container"
	"github.com/evgnomon/zygote/pkg/cert"
	"github.com/evgnomon/zygote/pkg/utils"
)

const defaultShardSize = 3
const hostNetworkName = "host"
const redisImage = "redis:7.4.2"
const defaultRedisPort = 6373
const memShortName = "mem"
const caCertPath = "/etc/certs/mem-ca-cert.pem"
const certPath = "/etc/certs/mem-server-cert.pem"
const keyCertPath = "/etc/certs/mem-server-key.pem"

var logger = utils.NewLogger()
var defaultRedisPortStr = strconv.Itoa(defaultRedisPort)

type MemNode struct {
	Tenant      string
	Domain      string
	NetworkName string
	ShardSize   int
	NumShards   int
	ShardIndex  int
	RepIndex    int
}

func NewMemNode() *MemNode {
	m := MemNode{}
	m.NetworkName = hostNetworkName
	m.ShardSize = defaultShardSize
	return &m
}

func (m *MemNode) containerName(name string) string {
	return utils.NodeContainer(name, m.Tenant, m.RepIndex, m.ShardIndex)
}

func (m *MemNode) sqlContainerName() string {
	return m.containerName(memShortName)
}

func (m *MemNode) certVolName() string {
	return fmt.Sprintf("%s-conf-cert", m.sqlContainerName())
}

func (m *MemNode) makeCertsVolume() {
	c, err := cert.Cert()
	logger.FatalIfErr("Create cert service", err)

	container.Vol(m.Tenant, c.CaCertPublic(), m.certVolName(),
		"/etc/certs", "mem-ca-cert.pem", container.AppNetworkName())
	container.Vol(m.Tenant, c.FunctionCertPublic(m.sqlContainerName()), m.certVolName(),
		"/etc/certs", "mem-server-cert.pem", container.AppNetworkName())
	container.Vol(m.Tenant, c.FunctionCertPrivate(m.sqlContainerName()), m.certVolName(),
		"/etc/certs", "mem-server-key.pem", container.AppNetworkName())
}

func (m *MemNode) CreateReplica(ctx context.Context) {
	m.makeCertsVolume()
	config := container.ContainerConfig{
		Name:        utils.NodeContainer("mem", m.Tenant, m.RepIndex, m.ShardIndex),
		NetworkName: m.NetworkName,
		Image:       redisImage,
		HealthCommand: []string{
			"CMD",
			"redis-cli",
			"--tls",
			"-p",
			defaultRedisPortStr,
			"--cert",
			certPath,
			"--key",
			keyCertPath,
			"--cacert",
			caCertPath,
			"ping",
		},
		Bindings: []string{
			fmt.Sprintf("%s-data:/var/lib/redis", utils.NodeContainer("mem", m.Tenant, m.RepIndex, m.ShardIndex)),
			fmt.Sprintf("%s:/etc/certs", m.certVolName()),
		},
		Caps:    []string{},
		EnvVars: []string{},
		Cmd: []string{
			"redis-server",
			"--port",
			"0",
			"--tls-port",
			defaultRedisPortStr,
			"--cluster-enabled",
			"yes",
			"--cluster-node-timeout",
			"5000",
			"--tls-cert-file",
			certPath,
			"--tls-key-file",
			keyCertPath,
			"--tls-ca-cert-file",
			caCertPath,
			"--tls-auth-clients",
			"yes",
			"--tls-replication",
			"yes",
			"--tls-cluster",
			"yes",
			"--tls-protocols",
			"TLSv1.2 TLSv1.3",
			"--tls-ciphers",
			"HIGH:!aNULL:!MD5",
			"--tls-ciphersuites",
			"TLS_AES_256_GCM_SHA384:TLS_AES_128_GCM_SHA256",
			"--appendonly",
			"yes",
		},
		Ports: map[int]int{
			defaultRedisPort + m.RepIndex*10 + m.ShardIndex*100: defaultRedisPort,
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
	cmd := []string{
		"redis-cli",
		"--tls",
		"--cert",
		certPath,
		"--key",
		keyCertPath,
		"--cacert",
		caCertPath,
		"--cluster",
		"create",
	}
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
	if m.RepIndex != 0 || m.ShardIndex != 0 {
		return
	}
	logger.Debug("Creating Redis cluster", utils.M{"replicaIndex": m.RepIndex, "shardIndex": m.ShardIndex})
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
		return container.SpawnAndWait(ctx, client, redisImage, m.Tenant,
			m.createRedisClusterCommand(), portMap,
			map[string]string{m.certVolName(): "/etc/certs"},
			m.NetworkName,
		)
	})
	logger.FatalIfErr("Create redis cluster", err)
}
