package mem

import (
	"fmt"
	"time"

	"github.com/evgnomon/zygote/pkg/cert"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/redis/go-redis/v9"
)

const defaultReplica = 0
const redisTimeout = 5 * time.Second
const targetReadPort = 6373

var logger = utils.NewLogger()

type MemEndpoint struct {
	Index int
	Host  string
	Port  int
}

func (e *MemEndpoint) Endpoint() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// MemEndpoints generates shard endpoints
func MemEndpoints(network, domain string, numShards,
	targetPort int) ([]MemEndpoint, error) {
	if numShards <= 0 {
		return nil, fmt.Errorf("shard count must be positive")
	}
	if targetPort <= 0 {
		return nil, fmt.Errorf("base ports must be positive")
	}
	endpoints := make([]MemEndpoint, numShards)
	for shardIndex := 0; shardIndex < numShards; shardIndex++ {
		endpoints[shardIndex] = MemEndpoint{
			Index: shardIndex,
			// Host:  utils.NodeHost(network, domain, defaultReplica, shardIndex),
			Host: "my.zygote.run",
			Port: utils.NodePort(network, targetPort, defaultReplica, shardIndex),
		}
	}
	return endpoints, nil
}

func Client() (*redis.ClusterClient, error) {
	tlsConfig := cert.TLSConfig(utils.HostName())
	if !utils.IsHostNetwork() {
		tlsConfig.InsecureSkipVerify = true //nolint:gosec
	}
	ep, err := MemEndpoints(utils.NetworkName(), utils.DomainName(), 2, targetReadPort)

	var addrs []string
	for _, e := range ep {
		addrs = append(addrs, e.Endpoint())
	}
	logger.Debug("Redis endpoints", utils.M{"endpoints": addrs})
	client := redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:     addrs,
		TLSConfig: tlsConfig,
	})

	return client, err
}
