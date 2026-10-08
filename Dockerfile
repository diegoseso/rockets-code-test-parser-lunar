# syntax=docker/dockerfile:1

# Build stage: generate protobuf code and compile the server.
FROM golang:1.26-alpine AS build
WORKDIR /app

# Code generation tools, pinned to the versions used in go.mod.
RUN go install github.com/bufbuild/buf/cmd/buf@v1.73.0 \
 && go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12 \
 && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2 \
 && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.31.0

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN cd proto && PATH="$(go env GOPATH)/bin:$PATH" buf generate --path gateway
RUN CGO_ENABLED=0 go build -o /bin/rockets-server .

# Runtime stage: defaults listen on :8088 (HTTP) and :8000 (gRPC).
FROM alpine:3.21
RUN adduser -D -u 10001 rockets
USER rockets
COPY --from=build /bin/rockets-server /usr/local/bin/rockets-server
EXPOSE 8088 8000
ENTRYPOINT ["rockets-server"]
