package container

import "context"

func InitRedisCluster() {
	jj := map[string]string{
		"6373": "6373",
	}
	client, err := CreateClinet()
	logger.FatalIfErr("Failed to create client", err)
	ctx := context.Background()
	Spawn(ctx, client, "redis:7.0.11", []string{
		"redis-cli", "--cluster", "create",
		"zygote-mem-shard-1:6373", "zygote-mem-shard-2:6373", "zygote-mem-shard-3:6373",
		"--cluster-replicas", "0", "--cluster-yes",
	}, jj, AppNetworkName())
}
