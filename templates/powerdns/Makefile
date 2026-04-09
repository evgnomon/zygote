.PHONY: podman clean all

all: podman

podman:
	podman-compose -f podman-compose.yaml up -d

clean:
	podman-compose -f podman-compose.yaml down -v
