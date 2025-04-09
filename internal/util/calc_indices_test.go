package util

import "testing"

// TestCalculateIndices tests the CalculateIndices function with various inputs
func TestCalculateIndices(t *testing.T) {
	tests := []struct {
		name           string
		hostname       string
		wantRepIndex   int
		wantShardIndex int
		wantErr        bool
	}{
		// Valid cases from original table
		{"shard-a basic", "shard-a.zygote.run", 0, 0, false},
		{"shard-b with number", "shard-b-1.zygote.run", 1, 1, false},
		{"shard-c with number", "shard-c-2.zygote.run", 2, 2, false},

		// Extended letter cases
		{"shard-d basic", "shard-d.zygote.run", 3, 0, false},
		{"shard-e with number", "shard-e-3.zygote.run", 4, 3, false},
		{"shard-z max letter", "shard-z-5.zygote.run", 25, 5, false},

		// Case insensitivity
		{"uppercase A", "shard-A.zygote.run", 0, 0, false},
		{"mixed case", "shard-B-1.zygote.run", 1, 1, false},

		// Sub domain
		{"uppercase A", "shard-A.foo.zygote.run", 0, 0, false},
		{"mixed case", "shard-B-1.foo.zygote.run", 1, 1, false},

		// Error cases
		{"empty string", "", 0, 0, true},
		{"missing domain", "shard-a", 0, 0, true},
		{"wrong prefix", "node-a.zygote.run", 0, 0, true},
		{"no letter", "shard-.zygote.run", 0, 0, true},
		{"invalid letter", "shard-1.zygote.run", 0, 0, true},
		{"invalid number", "shard-a-xyz.zygote.run", 0, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CalculateIndices(tt.hostname)

			// Check error condition
			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateIndices(%q) error = %v, wantErr %v", tt.hostname, err, tt.wantErr)
				return
			}

			// If we expect an error, don't check the results
			if tt.wantErr {
				return
			}

			// Check results
			if got.RepIndex != tt.wantRepIndex {
				t.Errorf("CalculateIndices(%q) RepIndex = %d, want %d", tt.hostname, got.RepIndex, tt.wantRepIndex)
			}
			if got.ShardIndex != tt.wantShardIndex {
				t.Errorf("CalculateIndices(%q) ShardIndex = %d, want %d", tt.hostname, got.ShardIndex, tt.wantShardIndex)
			}
		})
	}
}
