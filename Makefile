SHELL := /bin/bash

# Generate Go code from the proto definitions.
# Requires buf, protoc-gen-go, protoc-gen-go-grpc and
# protoc-gen-grpc-gateway on PATH (see README).
.PHONY: proto
proto:
	@echo Generating protobuf files...
	@cd proto && buf generate --path gateway

# Build and run the service in Docker (HTTP on :8088, gRPC on :8000).
.PHONY: run
run:
	@docker compose up --build

# Run locally (requires Go and generated protobuf code).
.PHONY: run-local
run-local: proto
	@go run main.go

.PHONY: test
test:
	@go test ./...

# The rockets test binary is provided by Lunar's ZIP and is not committed.
# Place it under cmd/ (e.g. cmd/darwin_arm64/rockets) or set ROCKETS_BIN.
ROCKETS_BIN ?= ./cmd/darwin_arm64/rockets
.PHONY: run-rockets
run-rockets:
	@$(ROCKETS_BIN) launch "http://localhost:8088/messages" --message-delay=500ms --concurrency-level=1
