package cert

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFunctionCertFileByContainer(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		domainEnv     string
		hostEnv       string
		expected      string
	}{
		{
			name:          "basic container cert file with default values",
			containerName: "my-container",
			domainEnv:     "",
			hostEnv:       "",
			expected:      "my-container.my.zygote.run",
		},
		{
			name:          "container cert file with custom domain",
			containerName: "test-service",
			domainEnv:     "custom.domain.com",
			hostEnv:       "",
			expected:      "test-service.custom.domain.com",
		},
		{
			name:          "container cert file with custom host",
			containerName: "api",
			domainEnv:     "",
			hostEnv:       "sql-a.my.zygote.run",
			expected:      "sql-api-a.my.zygote.run",
		},
		{
			name:          "container cert file with both custom domain and host",
			containerName: "worker",
			domainEnv:     "test.example.org",
			hostEnv:       "mem-b-1.test.example.org",
			expected:      "mem-worker-b-1.test.example.org",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			oldDomain := os.Getenv("Z_DOMAIN")
			oldHost := os.Getenv("Z_HOST")
			oldConfigHome := os.Getenv("ZYGOTE_CONFIG_HOME")

			defer func() {
				os.Setenv("Z_DOMAIN", oldDomain)
				os.Setenv("Z_HOST", oldHost)
				os.Setenv("ZYGOTE_CONFIG_HOME", oldConfigHome)
			}()

			if tt.domainEnv != "" {
				os.Setenv("Z_DOMAIN", tt.domainEnv)
			} else {
				os.Unsetenv("Z_DOMAIN")
			}

			if tt.hostEnv != "" {
				os.Setenv("Z_HOST", tt.hostEnv)
			} else {
				os.Unsetenv("Z_HOST")
			}

			// Use a temporary directory for testing
			tempDir := t.TempDir()
			os.Setenv("ZYGOTE_CONFIG_HOME", tempDir)

			certService, err := Cert()
			if err != nil {
				t.Fatalf("Failed to create cert service: %v", err)
			}

			result := certService.FunctionCertFileByContainer(tt.containerName)

			// Extract the filename from the full path to compare
			filename := filepath.Base(result)
			expected := tt.expected + "_cert.pem"

			if filename != expected {
				t.Errorf("FunctionCertFileByContainer(%q) = %q, want filename %q", tt.containerName, filename, expected)
			}

			// Verify the path structure is correct
			expectedDir := filepath.Join(tempDir, "certs", "functions", tt.expected)
			actualDir := filepath.Dir(result)

			if actualDir != expectedDir {
				t.Errorf("FunctionCertFileByContainer(%q) directory = %q, want %q", tt.containerName, actualDir, expectedDir)
			}
		})
	}
}

func TestFunctionCertFileByHost(t *testing.T) {
	tests := []struct {
		name      string
		domainEnv string
		hostEnv   string
		expected  string
	}{
		{
			name:      "host cert file with default values",
			domainEnv: "",
			hostEnv:   "shard-a",
			expected:  "shard.my.zygote.run",
		},
		{
			name:      "host cert file with custom domain",
			domainEnv: "prod.company.com",
			hostEnv:   "",
			expected:  "prod.company.com",
		},
		{
			name:      "all empty",
			domainEnv: "",
			hostEnv:   "",
			expected:  "my.zygote.run",
		},
		{
			name:      "host cert file with custom host",
			domainEnv: "",
			hostEnv:   "sql-c",
			expected:  "sql.my.zygote.run",
		},
		{
			name:      "host cert file with custom host without letter",
			domainEnv: "",
			hostEnv:   "sql",
			expected:  "sql.my.zygote.run",
		},
		{
			name:      "host cert file with shared host",
			domainEnv: "",
			hostEnv:   "mem-d-2",
			expected:  "mem.my.zygote.run",
		},
		{
			name:      "host cert file with custom domain and host",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e",
			expected:  "api.staging.app.io",
		},
		{
			name:      "host cert file with custom domain and host and shard",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e-3",
			expected:  "api.staging.app.io",
		},
		{
			name:      "host cert file with shared host with full host",
			domainEnv: "",
			hostEnv:   "mem-d-2.my.zygote.run",
			expected:  "mem-d-2.my.zygote.run",
		},
		{
			name:      "host cert file with custom domain and host with full host",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e.my.zygote.run",
			expected:  "api.staging.app.io",
		},
		{
			name:      "host cert file with custom domain and host and shard with full host",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e-3.my.zygote.run",
			expected:  "api.staging.app.io",
		},
		{
			name:      "host cert file with custom domain and host with full host with correct host name",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e.staging.app.io",
			expected:  "api-e.staging.app.io",
		},
		{
			name:      "host cert file with custom domain and host and shard with full host with correct host name",
			domainEnv: "staging.app.io",
			hostEnv:   "api-e-3.staging.app.io",
			expected:  "api-e-3.staging.app.io",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup environment variables
			oldDomain := os.Getenv("Z_DOMAIN")
			oldHost := os.Getenv("Z_HOST")
			oldConfigHome := os.Getenv("ZYGOTE_CONFIG_HOME")

			defer func() {
				os.Setenv("Z_DOMAIN", oldDomain)
				os.Setenv("Z_HOST", oldHost)
				os.Setenv("ZYGOTE_CONFIG_HOME", oldConfigHome)
			}()

			if tt.domainEnv != "" {
				os.Setenv("Z_DOMAIN", tt.domainEnv)
			} else {
				os.Unsetenv("Z_DOMAIN")
			}

			if tt.hostEnv != "" {
				os.Setenv("Z_HOST", tt.hostEnv)
			} else {
				os.Unsetenv("Z_HOST")
			}

			// Use a temporary directory for testing
			tempDir := t.TempDir()
			os.Setenv("ZYGOTE_CONFIG_HOME", tempDir)

			certService, err := Cert()
			if err != nil {
				t.Fatalf("Failed to create cert service: %v", err)
			}

			result := certService.FunctionCertFileByHost()

			// Extract the filename from the full path to compare
			filename := filepath.Base(result)
			expected := tt.expected + "_cert.pem"

			if filename != expected {
				t.Errorf("FunctionCertFileByHost() = %q, want filename %q", filename, expected)
			}

			// Verify the path structure is correct
			expectedDir := filepath.Join(tempDir, "certs", "functions", tt.expected)
			actualDir := filepath.Dir(result)

			if actualDir != expectedDir {
				t.Errorf("FunctionCertFileByHost() directory = %q, want %q", actualDir, expectedDir)
			}
		})
	}
}
