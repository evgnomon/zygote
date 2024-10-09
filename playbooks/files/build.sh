#!/bin/sh

set -e

[ ! -d ./bin ] && mkdir ./bin

export GOFLAGS="-buildvcs=false"
go build -v -o ./bin ./cmd/zygote 
go build -v -o ./bin ./cmd/zygote-controller
