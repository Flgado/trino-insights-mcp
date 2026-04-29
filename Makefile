BINARY   := trino-insights-mcp
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT   ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
DATE     ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -s -w -X github.com/Flgado/trino-insights-mcp/internal/timcp.Version=$(VERSION)

.PHONY: build run test lint clean docker

build:
	CGO_ENABLED=0 go build -ldflags="$(LDFLAGS)" -o $(BINARY) ./cmd

run: build
	./$(BINARY) stdio

test:
	go test ./...

lint:
	golangci-lint run ./...

clean:
	rm -f $(BINARY)

docker:
	docker build -t $(BINARY):$(VERSION) .
