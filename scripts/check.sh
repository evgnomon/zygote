#!/bin/sh

set -e

export GOFLAGS="-buildvcs=false"
golangci-lint run
go test -v ./...
