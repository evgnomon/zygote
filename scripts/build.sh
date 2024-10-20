#!/bin/bash

set -e

[ ! -d ./dist ] && mkdir ./dist

export GOFLAGS="-buildvcs=false"

# Define target operating systems and architectures
TARGETS=(
    "darwin/amd64"
    "darwin/arm64"
    "linux/amd64"
    "linux/arm64"
    "windows/amd64"
    "windows/arm64"
)

# Iterate over target OS/Arch and build binaries
for TARGET in "${TARGETS[@]}"; do
    OS=$(echo $TARGET | cut -d '/' -f 1)
    ARCH=$(echo $TARGET | cut -d '/' -f 2)

    # Build zygote binary
    OUTPUT="./dist/zygote-$OS-$ARCH"
    [ "$OS" = "windows" ] && OUTPUT="$OUTPUT.exe"
    GOOS=$OS GOARCH=$ARCH go build -v -o "$OUTPUT" ./cmd/zygote

    # Build zygote-controller binary
    OUTPUT_CONTROLLER="./dist/zygote_controller-$OS-$ARCH"
    [ "$OS" = "windows" ] && OUTPUT_CONTROLLER="$OUTPUT_CONTROLLER.exe"
    GOOS=$OS GOARCH=$ARCH go build -v -o "$OUTPUT_CONTROLLER" ./cmd/zygote_controller
done
