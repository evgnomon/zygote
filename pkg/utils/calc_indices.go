package utils

import (
	"fmt"
	"strconv"
	"strings"
)

// ClusterIndex represents the calculated indices
type ClusterIndex struct {
	RepIndex   int
	ShardIndex int
}

// GetClusterIndex calculates rep-index and shard-index from hostname
func GetClusterIndex(hostname string) (ClusterIndex, error) {
	// Remove the domain part
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return ClusterIndex{}, fmt.Errorf("invalid hostname format")
	}
	shardPart := parts[0]

	// Split into shard letter and optional number
	shardComponents := strings.Split(shardPart, "-")
	if len(shardComponents) < 2 || shardComponents[0] != "shard" {
		return ClusterIndex{}, fmt.Errorf("hostname must start with 'shard-'")
	}

	// Determine rep-index based on letter position (a=0, b=1, c=2, d=3, etc.)
	if len(shardComponents[1]) < 1 {
		return ClusterIndex{}, fmt.Errorf("shard letter is required")
	}
	letter := strings.ToLower(shardComponents[1][0:1]) // Take first character and convert to lowercase
	if letter < "a" || letter > "z" {
		return ClusterIndex{}, fmt.Errorf("shard identifier must be a letter (a-z)")
	}
	repIndex := int(letter[0] - 'a') // Convert letter to 0-based index

	// Determine shard-index (0 if no number, otherwise parse the number)
	shardIndex := 0
	if len(shardComponents) > 2 {
		num, err := strconv.Atoi(shardComponents[2])
		if err != nil {
			return ClusterIndex{}, fmt.Errorf("invalid shard number: %v", err)
		}
		shardIndex = num
	}

	return ClusterIndex{
		RepIndex:   repIndex,
		ShardIndex: shardIndex,
	}, nil
}
