package mem

import (
	"context"
	"fmt"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
	dnet "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/evgnomon/zygote/internal/container"
)

const redisImage = "redis:7.4.2"
const hostNetworkName = "host"
const redisPort = 6373

type MemShard struct {
	Tenant      string
	Domain      string
	NetworkName string
	ShardSize   int
	NumShards   int
}

func NewMemShard(domain string) *MemShard {
	m := MemShard{}
	m.Domain = domain
	m.NetworkName = hostNetworkName
	m.ShardSize = 3
	m.Tenant = "zygote"
	return &m
}

func (m *MemShard) CreateReplica(repIndex int) error {
	ctx := context.Background()
	cli, err := container.CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	config := &dcontainer.Config{
		Image: redisImage,
		Cmd: []string{"redis-server", "--port", "6373",
			"--cluster-enabled", "yes", "--cluster-node-timeout", "5000", "--appendonly", "yes"},
		ExposedPorts: nat.PortSet{
			"6373/tcp": struct{}{},
		},
		Healthcheck: &dcontainer.HealthConfig{
			Test:     []string{"CMD", "redis-cli", "-p", "6373", "ping", "--raw", "incr", "ping"},
			Timeout:  20 * time.Second,
			Retries:  20,
			Interval: 1 * time.Second,
		},
	}
	hostConfig := &dcontainer.HostConfig{
		Binds: []string{
			fmt.Sprintf("%s-mem-shard-%d-data:/var/lib/redis", m.Tenant, repIndex+1),
		},
		RestartPolicy: dcontainer.RestartPolicy{
			Name: dcontainer.RestartPolicyAlways,
		},
	}

	if m.NetworkName != "" {
		hostConfig.NetworkMode = dcontainer.NetworkMode(m.NetworkName)
		if m.NetworkName != hostNetworkName {
			hostConfig.PortBindings = nat.PortMap{
				"6373/tcp": []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", redisPort+repIndex)},
				},
			}
			_, err = cli.NetworkInspect(ctx, m.NetworkName, dnet.InspectOptions{})
			if err != nil {
				_, err = cli.NetworkCreate(ctx, m.NetworkName, dnet.CreateOptions{})
			}
			if err != nil {
				return fmt.Errorf("failed to create network: %w", err)
			}
		}
	}
	container.Pull(ctx, redisImage)
	containerName := fmt.Sprintf("%s-mem-shard-%d", m.Tenant, repIndex+1)
	resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
	if err != nil {
		if errdefs.IsConflict(err) {
			return fmt.Errorf("container already exists: %s", containerName)
		}
		return fmt.Errorf("failed to create container: %w", err)
	}
	if err := cli.ContainerStart(ctx, resp.ID, dcontainer.StartOptions{}); err != nil {
		return fmt.Errorf("failed to start container: %w", err)
	}
	return nil
}

// redis-cli --cluster create shard-a.zygote.run:6373 shard-a-1.zygote.run:6373 shard-a-2.zygote.run:6373 --cluster-replicas 0 --cluster-yes
func (m *MemShard) createRedisClusterCommand() []string {
	// Base command parts
	cmd := []string{"redis-cli", "--cluster", "create"}
	// Generate shard hostnames based on shardSize
	for i := 0; i < m.ShardSize*m.NumShards; i++ {
		// Convert shard number to letter (a=0, b=1, c=2, etc.)
		shardLetter := string('a' + rune(i/m.NumShards))
		var host string
		if i%m.NumShards == 0 {
			host = fmt.Sprintf("shard-%s.%s:6373", shardLetter, m.Domain)
			cmd = append(cmd, host)
			continue
		}
		host = fmt.Sprintf("shard-%s-%d.%s:6373", shardLetter, i%m.NumShards, m.Domain)
		cmd = append(cmd, host)
	}

	// Add replica and confirmation flags
	cmd = append(cmd, "--cluster-replicas", fmt.Sprintf("%d", m.ShardSize-1), "--cluster-yes")
	return cmd
}

func (m *MemShard) Init(ctx context.Context) error {
	portMap := map[string]string{
		"6373": "6373",
	}
	client, err := container.CreateClinet()
	if err != nil {
		return fmt.Errorf("failed to create docker client: %w", err)
	}
	return container.SpawnAndWait(ctx, client, redisImage, m.createRedisClusterCommand(), portMap, hostNetworkName)
}

func CreateMemContainer(numShards int, networkName string) {
	ctx := context.Background()
	cli, err := container.CreateClinet()
	if err != nil {
		panic(err)
	}
	for i := 1; i <= numShards; i++ {
		config := &dcontainer.Config{
			Image: redisImage,
			Cmd:   []string{"redis-server", "--port", "6373", "--cluster-enabled", "yes", "--cluster-node-timeout", "5000", "--appendonly", "yes"},
			ExposedPorts: nat.PortSet{
				"6373/tcp": struct{}{},
			},
			Healthcheck: &dcontainer.HealthConfig{
				Test:     []string{"CMD", "redis-cli", "-p", "6373", "ping", "--raw", "incr", "ping"},
				Timeout:  20 * time.Second,
				Retries:  20,
				Interval: 1 * time.Second,
			},
		}

		hostConfig := &dcontainer.HostConfig{
			PortBindings: nat.PortMap{
				"6373/tcp": []nat.PortBinding{
					{HostIP: "0.0.0.0", HostPort: fmt.Sprintf("%d", 6373+i-1)},
				},
			},
			Binds: []string{
				fmt.Sprintf("zygote-mem-shard-%d-data:/var/lib/redis", i),
			},
			RestartPolicy: dcontainer.RestartPolicy{
				Name: dcontainer.RestartPolicyAlways,
			},
		}

		_, err = cli.NetworkInspect(ctx, networkName, dnet.InspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, networkName, dnet.CreateOptions{})
		}
		if err != nil {
			panic(err)
		}

		if networkName != "" {
			hostConfig.NetworkMode = dcontainer.NetworkMode(networkName)
		}

		container.Pull(ctx, redisImage)
		containerName := fmt.Sprintf("zygote-mem-shard-%d", i)
		resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, containerName)
		if err != nil {
			if errdefs.IsConflict(err) {
				fmt.Printf("Container already exists: %s\n", containerName)
				return
			}
			panic(err)
		}

		if err := cli.ContainerStart(ctx, resp.ID, dcontainer.StartOptions{}); err != nil {
			panic(err)
		}
	}
}
