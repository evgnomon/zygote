package util

import (
	"fmt"
	"strconv"
	"strings"
)

// HostIndices represents the calculated indices
type HostIndices struct {
	RepIndex   int
	ShardIndex int
}

// CalculateIndices calculates rep-index and shard-index from hostname
func CalculateIndices(hostname string) (HostIndices, error) {
	// Remove the domain part
	parts := strings.Split(hostname, ".")
	if len(parts) < 2 {
		return HostIndices{}, fmt.Errorf("invalid hostname format")
	}
	shardPart := parts[0]

	// Split into shard letter and optional number
	shardComponents := strings.Split(shardPart, "-")
	if len(shardComponents) < 2 || shardComponents[0] != "shard" {
		return HostIndices{}, fmt.Errorf("hostname must start with 'shard-'")
	}

	// Determine rep-index based on letter position (a=0, b=1, c=2, d=3, etc.)
	if len(shardComponents[1]) < 1 {
		return HostIndices{}, fmt.Errorf("shard letter is required")
	}
	letter := strings.ToLower(shardComponents[1][0:1]) // Take first character and convert to lowercase
	if letter < "a" || letter > "z" {
		return HostIndices{}, fmt.Errorf("shard identifier must be a letter (a-z)")
	}
	repIndex := int(letter[0] - 'a') // Convert letter to 0-based index

	// Determine shard-index (0 if no number, otherwise parse the number)
	shardIndex := 0
	if len(shardComponents) > 2 {
		num, err := strconv.Atoi(shardComponents[2])
		if err != nil {
			return HostIndices{}, fmt.Errorf("invalid shard number: %v", err)
		}
		shardIndex = num
	}

	return HostIndices{
		RepIndex:   repIndex,
		ShardIndex: shardIndex,
	}, nil
}
