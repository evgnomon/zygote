/*
Copyright (C) 2025- Hamed Ghasemzadeh. All rights reserved.
License: HGL General License <https://evgnomon.org/docs/hgl>
*/
package utils

import (
	"os"
	"testing"
)

func TestNodeSuffix(t *testing.T) {
	tests := []struct {
		name   string
		host   string
		domain string
		want   string
	}{
		{"host without domain suffix", "myhost", "example.com", ""},
		{"host with domain suffix but no hyphen", "myhost.example.com", "example.com", ""},
		{"host with domain suffix and single hyphen", "shard-a.example.com", "example.com", "a"},
		{"host with domain suffix and multiple hyphens", "shard-a-1.example.com", "example.com", "a-1"},
		{"host with multiple parts and hyphens", "cluster-node-b-2.example.com", "example.com", "b-2"},
		{"empty host", "", "example.com", ""},
		{"empty domain", "myhost.example.com", "", ""},
		{"host equals domain", "example.com", "example.com", ""},
		{"host with wrong domain", "myhost.other.com", "example.com", ""},
		{"complex case with tenant", "tenant-service-c-3.my.domain.com", "my.domain.com", "c-3"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nodeSuffix(tt.host, tt.domain)
			if got != tt.want {
				t.Errorf("nodeSuffix(%q, %q) = %q, want %q", tt.host, tt.domain, got, tt.want)
			}
		})
	}
}

func TestContainerCertName(t *testing.T) {
	tests := []struct {
		name          string
		containerName string
		want          string
		domain        string
	}{
		{"basic container name", "my-container", "my-container", ""},
		{"basic container name", "my-container-a", "my-container-a", ""},
		{"basic container name", "my-container-a-1", "my-container-a-1", ""},
		{"basic container name", "zygote-my-container-a-1", "zygote-my-container-a-1.example.com", "example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.domain != "" {
				d := os.Getenv("Z_DOMAIN")
				defer os.Setenv("Z_DOMAIN", d)
				os.Setenv("Z_DOMAIN", tt.domain)
			}
			got := ContainerCertName(tt.containerName)
			if got != tt.want {
				t.Errorf("ContainerCertName(%q) = %q, want %q", tt.containerName, got, tt.want)
			}
		})
	}
}
