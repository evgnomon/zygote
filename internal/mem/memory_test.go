package mem

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestMemShard_createRedisClusterCommand(t *testing.T) {
	tests := []struct {
		name     string
		memShard *MemNode
		want     []string
	}{
		{
			name: "Single shard with one replica",
			memShard: &MemNode{
				ShardSize:   2,
				NumShards:   1,
				Domain:      "example.com",
				Tenant:      "zygote",
				NetworkName: "host",
			},
			want: []string{
				"redis-cli",
				"--tls",
				"--cert",
				"/etc/certs/mem-server-cert.pem",
				"--key",
				"/etc/certs/mem-server-key.pem",
				"--cacert",
				"/etc/certs/mem-ca-cert.pem",
				"--cluster", "create",
				"shard-a.example.com:6373",
				"shard-b.example.com:6373",
				"--cluster-replicas", "1",
				"--cluster-yes",
			},
		},
		{
			name: "two shard with single master",
			memShard: &MemNode{
				ShardSize:   1,
				NumShards:   2,
				Domain:      "example.com",
				Tenant:      "zygote",
				NetworkName: "host",
			},
			want: []string{
				"redis-cli",
				"--tls",
				"--cert",
				"/etc/certs/mem-server-cert.pem",
				"--key",
				"/etc/certs/mem-server-key.pem",
				"--cacert",
				"/etc/certs/mem-ca-cert.pem",
				"--cluster", "create",
				"shard-a.example.com:6373",
				"shard-a-1.example.com:6373",
				"--cluster-replicas", "0",
				"--cluster-yes",
			},
		},
		{
			name: "Two shards with one replica",
			memShard: &MemNode{
				ShardSize:   2,
				NumShards:   2,
				Domain:      "test.com",
				Tenant:      "zygote",
				NetworkName: "host",
			},
			want: []string{
				"redis-cli",
				"--tls",
				"--cert",
				"/etc/certs/mem-server-cert.pem",
				"--key",
				"/etc/certs/mem-server-key.pem",
				"--cacert",
				"/etc/certs/mem-ca-cert.pem",
				"--cluster", "create",
				"shard-a.test.com:6373",
				"shard-a-1.test.com:6373",
				"shard-b.test.com:6373",
				"shard-b-1.test.com:6373",
				"--cluster-replicas", "1",
				"--cluster-yes",
			},
		},
		{
			name: "Three shards with two replicas",
			memShard: &MemNode{
				ShardSize:   3,
				NumShards:   3,
				Domain:      "cluster.local",
				Tenant:      "zygote",
				NetworkName: "host",
			},
			want: []string{
				"redis-cli",
				"--tls",
				"--cert",
				"/etc/certs/mem-server-cert.pem",
				"--key",
				"/etc/certs/mem-server-key.pem",
				"--cacert",
				"/etc/certs/mem-ca-cert.pem",
				"--cluster", "create",
				"shard-a.cluster.local:6373",
				"shard-a-1.cluster.local:6373",
				"shard-a-2.cluster.local:6373",
				"shard-b.cluster.local:6373",
				"shard-b-1.cluster.local:6373",
				"shard-b-2.cluster.local:6373",
				"shard-c.cluster.local:6373",
				"shard-c-1.cluster.local:6373",
				"shard-c-2.cluster.local:6373",
				"--cluster-replicas", "2",
				"--cluster-yes",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.memShard
			got := m.createRedisClusterCommand()
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("createRedisClusterCommand() failed:\nExpected: %v\nGot: %v\nDiff (-want +got):\n%s", tt.want, got, diff)
			}
		})
	}
}
