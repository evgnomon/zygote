package utils

import (
	"fmt"
	"os"
)

const defaultNetworkName = "mynet"
const hostNetworkName = "host"
const maxShardSize = 9
const myZygoteDomain = "my.zygote.run"
const networkNameEnvVar = "DOCKER_NETWORK_NAME"

func NodePort(network string, targetPort, replicaIndex, shardIndex int) int {
	if network == hostNetworkName {
		return targetPort
	}
	if replicaIndex < 0 || shardIndex < 0 {
		fmt.Println("replicaIndex and shardIndex must be non-negative")
		os.Exit(1)
	}
	if replicaIndex >= maxShardSize {
		fmt.Println("replicaIndex must be less than 10")
		os.Exit(1)
	}
	return targetPort + replicaIndex*10 + 100*shardIndex
}

func NodeHost(network, domain string, replicaIndex, shardIndex int) string {
	shardName := string('a' + rune(replicaIndex))
	if network != hostNetworkName {
		return "127.0.0.1"
	}
	if shardIndex > 0 {
		return fmt.Sprintf("shard-%s-%d.%s", shardName, shardIndex, domain)
	} else {
		return fmt.Sprintf("shard-%s.%s", shardName, domain)
	}
}

func NodeContainer(nodeType, tenant string, replicaIndex, shardIndex int) string {
	if shardIndex == 0 {
		return fmt.Sprintf("%s-%s-%s", tenant, nodeType, string('a'+rune(replicaIndex)))
	}
	return fmt.Sprintf("%s-%s-%s-%d", tenant, nodeType, string('a'+rune(replicaIndex)), shardIndex)
}

func DomainName() string {
	domain := os.Getenv("Z_DOMAIN")
	if domain == "" {
		domain = myZygoteDomain
	}
	return domain
}

func HostName() string {
	domain := os.Getenv("Z_HOST")
	if domain == "" {
		domain = myZygoteDomain
	}
	return domain
}

func TenantName() string {
	domain := os.Getenv("Z_TENANT")
	if domain == "" {
		domain = "zygote"
	}
	return domain
}

func NetworkName() string {
	if os.Getenv(networkNameEnvVar) != "" {
		return os.Getenv(networkNameEnvVar)
	}
	return defaultNetworkName
}

func IsHostNetwork() bool {
	return NetworkName() == hostNetworkName
}
