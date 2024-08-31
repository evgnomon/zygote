package mem

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types"
	dcontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/evgnomon/zygote/internal/container"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const redisImage = "redis:7.0.11"

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
		}

		_, err = cli.NetworkInspect(ctx, networkName, types.NetworkInspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, networkName, types.NetworkCreate{})
		}
		if err != nil {
			panic(err)
		}

		if networkName != "" {
			hostConfig.NetworkMode = dcontainer.NetworkMode(networkName)
		}

		container.Pull(ctx, redisImage)
		resp, err := cli.ContainerCreate(ctx, config, hostConfig, nil, nil, fmt.Sprintf("zygote-mem-shard-%d", i))
		if err != nil {
			panic(err)
		}

		if err := cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}); err != nil {
			panic(err)
		}
	}
}