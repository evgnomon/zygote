package utils

import (
	"fmt"
	"os"
)

const hostNetworkName = "host"
const maxShardSize = 9
const myZygoteDomain = "my.zygote.run"

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

func NodeContainer(nodeType, tenant string, replicaIndex, shardIndex int) string {
	if shardIndex == 0 {
		return fmt.Sprintf("%s-%s-%s", tenant, nodeType, string('a'+rune(replicaIndex)))
	}
	return fmt.Sprintf("%s-%s-%s-%d", tenant, nodeType, string('a'+rune(replicaIndex)), shardIndex)
}

func NodeEndpoint(network, nodeType, tenant, domain string, targetPort, replicaIndex, shardIndex int) string {
	shardName := string('a' + rune(replicaIndex))
	if network != hostNetworkName {
		return fmt.Sprintf("%s:%d",
			NodeContainer(nodeType, tenant, replicaIndex, shardIndex),
			NodePort(network, targetPort, replicaIndex, shardIndex))
	}
	if shardIndex > 0 {
		return fmt.Sprintf("shard-%s-%d.%s:%d", shardName, shardIndex, domain, targetPort)
	} else {
		return fmt.Sprintf("shard-%s.%s:%d", shardName, domain, targetPort)
	}
}

func NodeHost(network, nodeType, tenant, domain string, replicaIndex, shardIndex int) string {
	shardName := string('a' + rune(replicaIndex))
	if network != hostNetworkName {
		return NodeContainer(nodeType, tenant, replicaIndex, shardIndex)
	}
	if shardIndex > 0 {
		return fmt.Sprintf("shard-%s-%d.%s", shardName, shardIndex, domain)
	} else {
		return fmt.Sprintf("shard-%s.%s", shardName, domain)
	}
}

func RemoteHost(network, domain string, replicaIndex, shardIndex int) string {
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
