package db

import (
	"context"
	"fmt"
	"time"

	dcontainer "github.com/docker/docker/api/types/container"
	networktypes "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/errdefs"
	"github.com/docker/go-connections/nat"
	"github.com/evgnomon/zygote/internal/container"
)

const mysqlImage = "mysql:8.0.33"

func CreateDBContainer(numShards int, networkName string) {
	ctx := context.Background()
	cli, err := container.CreateClinet()
	if err != nil {
		panic(err)
	}

	envVars := []string{
		"MYSQL_ROOT_PASSWORD=root1234",
	}

	for i := 1; i <= numShards; i++ {
		config := &dcontainer.Config{
			Image: mysqlImage,
			Env:   envVars,
			ExposedPorts: nat.PortSet{
				"3306": struct{}{},
			},
			Healthcheck: &dcontainer.HealthConfig{
				Test: []string{"CMD",
					"mysql",
					"-h",
					"localhost",
					"-u",
					fmt.Sprintf("test_%d", i),
					"-ppassword",
					"-e",
					"SHOW tables;",
					fmt.Sprintf("myproject_%d", i),
				},
				Timeout:  20 * time.Second,
				Retries:  20,
				Interval: 1 * time.Second,
			},
		}

		hostConfig := &dcontainer.HostConfig{
			PortBindings: nat.PortMap{
				"3306": []nat.PortBinding{
					{
						HostIP:   "0.0.0.0",
						HostPort: fmt.Sprintf("%d", 3306+i-1),
					},
				},
			},
			Binds: []string{
				fmt.Sprintf("zygote-db-%d-data:/var/lib/mysql", i),
				fmt.Sprintf("zygote-db-conf-%d:/docker-entrypoint-initdb.d", i),
			},
			CapAdd: []string{"SYS_NICE"},
		}

		_, err = cli.NetworkInspect(ctx, networkName, networktypes.InspectOptions{})
		if err != nil {
			_, err = cli.NetworkCreate(ctx, networkName, networktypes.CreateOptions{})
		}
		if err != nil {
			panic(err)
		}

		if networkName != "" {
			hostConfig.NetworkMode = dcontainer.NetworkMode(networkName)
		}

		container.Pull(ctx, mysqlImage)
		containerName := fmt.Sprintf("zygote-db-shard-%d", i)
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
