package mem

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"log"

	"github.com/evgnomon/zygote/pkg/mem"
	"github.com/evgnomon/zygote/pkg/utils"
	"github.com/redis/go-redis/v9"
)

// CompressJSON compresses a Go value into gzip-compressed JSON bytes
func CompressJSON(data any) ([]byte, error) {
	// Marshal to JSON
	jsonBytes, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	// Compress with gzip
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	_, err = gw.Write(jsonBytes)
	if err != nil {
		return nil, err
	}
	err = gw.Close()
	if err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecompressJSON decompresses gzip-compressed JSON bytes into a Go value
func DecompressJSON(compressed []byte, output any) error {
	gr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return err
	}
	defer gr.Close()

	var decompressed bytes.Buffer
	_, err = decompressed.ReadFrom(gr)
	if err != nil {
		return err
	}

	return json.Unmarshal(decompressed.Bytes(), output)
}

// StoreInRedis stores data in Redis
func StoreInRedis(client *redis.ClusterClient, ctx context.Context, key string, value []byte) error {
	return client.Set(ctx, key, value, 0).Err()
}

// RetrieveFromRedis retrieves data from Redis
func RetrieveFromRedis(client *redis.ClusterClient, ctx context.Context, key string) ([]byte, error) {
	return client.Get(ctx, key).Bytes()
}

func RunExample() {
	// Initialize Redis client
	ctx := context.Background()
	client, err := mem.Client()
	logger.FatalIfErr("Create Redis client", err)

	// Sample data
	data := map[string]any{
		"key":     "value",
		"numbers": []int{1, 2, 3, 4, 5},
	}

	// Compress the data
	compressed, err := CompressJSON(data)
	if err != nil {
		log.Fatal("Failed to compress JSON:", err)
	}

	// Store in Redis
	err = StoreInRedis(client, ctx, "mykey", compressed)
	if err != nil {
		log.Fatal("Failed to store in Redis:", err)
	}

	// Retrieve from Redis
	retrieved, err := RetrieveFromRedis(client, ctx, "mykey")
	if err != nil {
		log.Fatal("Failed to retrieve from Redis:", err)
	}

	// Decompress and unmarshal
	var originalData map[string]any
	err = DecompressJSON(retrieved, &originalData)
	if err != nil {
		log.Fatal("Failed to decompress JSON:", err)
	}

	logger.Info("Decompressed data", utils.M{"originalData": originalData})
}
