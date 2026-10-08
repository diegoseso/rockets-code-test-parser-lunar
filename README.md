# Rockets — Backend Engineer Challenge (2026 delivery)

A Go service that consumes rocket state-change messages and exposes the
current state of every rocket through a REST API. The test program posts
messages to `http://localhost:8088/messages`; the state is rebuilt per
channel in `messageNumber` order, with at-least-once and out-of-order
delivery handled explicitly.

If you are looking for information about the project and the AI usage please see AI_WORKFLOW.md

## Running the service

### With Docker

```bash
docker compose up --build
```

This builds the image (protobuf generation included) and starts the
service. HTTP listens on `:8088`, gRPC on `:8000`.

### Locally

Prerequisites: Go 1.26+, `buf`, and the protoc plugins
(`protoc-gen-go`, `protoc-gen-go-grpc`, `protoc-gen-grpc-gateway`) on
`PATH`:

```bash
go install github.com/bufbuild/buf/cmd/buf@v1.73.0
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@v2.31.0
```

Then generate the protobuf code and run:

```bash
make proto        # or: cd proto && buf generate --path gateway
go run main.go
```

No configuration file is needed: the service defaults to HTTP `:8088`
and gRPC `:8000`. To override, copy `rockets_gateway.yml.dist` to
`rockets_gateway.yml` and edit, or pass `--http` / `--grpc-server` flags.

### Sending test messages

Under the cmd folder you will find a copy of the rockets binary provided for different architectures.

```bash
./rockets launch "http://localhost:8088/messages" --message-delay=500ms --concurrency-level=1
```

### Quick check with curl

```bash
curl -X POST http://localhost:8088/messages -H 'Content-Type: application/json' -d '{
  "metadata": {
    "channel": "193270a9-c9cf-404a-8f83-838e71d9ae67",
    "messageNumber": 1,
    "messageTime": "2022-02-02T19:39:05.86337+01:00",
    "messageType": "RocketLaunched"
  },
  "message": {"type": "Falcon-9", "launchSpeed": 500, "mission": "ARTEMIS"}
}'

curl http://localhost:8088/v1/rockets/193270a9-c9cf-404a-8f83-838e71d9ae67
```

## API

| Method | Endpoint | Description |
|--------|----------|-------------|
| `POST` | `/messages` | Ingest one rocket event message |
| `GET`  | `/v1/rockets/{id}` | Current state of one rocket |
| `POST` | `/v1/rockets/list` | List all rockets, with optional filtering and sorting |

### `POST /messages`

Request body: the message envelope produced by the test program
(`metadata` + `message`, see the curl example above). The five message
types are `RocketLaunched`, `RocketSpeedIncreased`,
`RocketSpeedDecreased`, `RocketExploded` and `RocketMissionChanged`.

Responses:

- `200 OK` message was applied, was a duplicate, conflicted with an
  already-seen message number, was buffered pending earlier numbers, or
  was permanently invalid (see below). Empty body.
- non-2xx — a transient failure; the test program redelivers.

### `GET /v1/rockets/{id}`

```bash
curl http://localhost:8088/v1/rockets/193270a9-c9cf-404a-8f83-838e71d9ae67
```

```json
{
  "rocket": {
    "id": "193270a9-c9cf-404a-8f83-838e71d9ae67",
    "type": "Falcon-9",
    "mission": "ARTEMIS",
    "speed": 500,
    "status": "launched",
    "launchedAt": "2022-02-02T18:39:05.863370Z",
    "lastUpdatedAt": "2022-02-02T18:39:05.863370Z"
  }
}
```

`status` is `launched` or `exploded`; `explodedReason` is only present
when exploded. Unknown rockets return `404`.

### `POST /v1/rockets/list`

```bash
curl -X POST http://localhost:8088/v1/rockets/list -H 'Content-Type: application/json' -d '{
  "orderBy": "speed",
  "order": "desc"
}'
```

```json
{
  "rockets": [
    {"id": "...", "type": "Falcon-9", "mission": "ARTEMIS", "speed": 1500, "status": "launched", "...": "..."}
  ],
  "total": 1
}
```

- `orderBy`: `id` (default), `speed`, `mission` or `status`.
- `order`: `asc` (default) or `desc`. Ties are broken by `id`, so the
  order is always deterministic.
- Optional filters: `filterId` (exact match), `filterMinSpeed`,
  `filterMaxSpeed`.

## Design decisions and trade-offs

1. **Ordering by `messageNumber` with a per-channel buffer.** State is
   rebuilt by applying messages in contiguous `messageNumber` order, not
   arrival order. Each channel keeps an inbox of accepted messages; a
   message is applied only when every lower number has been applied. The
   launch is expected to be message #1 — if it has not arrived, later
   messages stay pending instead of fabricating a launch. After a
   `RocketExploded`, higher-numbered messages are accepted but do not
   mutate speed or mission; because application is contiguous, anything
   numbered below the explosion was necessarily applied before it.
2. **Idempotency and "first wins".** Every accepted message is recorded
   per channel by `messageNumber`. A redelivery with the same payload is
   a duplicate and does not mutate state. The same number arriving with
   a *different* payload is a conflict: the statement gives no criterion
   to pick a winner, so the first message received wins and the conflict
   is logged.
3. **2xx on duplicates and permanently invalid messages.** The test
   program redelivers on any non-2xx, so messages that can never become
   useful; duplicates, conflicts, missing metadata, unknown type,
   `messageNumber < 1`, missing payload — are acknowledged and discarded
   (with a log line), never retried.
4. **Non-2xx only on transient failure.** The ingestion endpoint returns
   an error only when the store itself fails transiently, which is the
   only case where redelivery helps. The in-memory store does not
   produce such failures, but the path exists so a persistent store can
   be dropped in without changing the contract.
5. **In-memory store.** A single process holds all state in memory. A
   restart loses every rocket and every buffered message; the test
   program would rebuild state only for messages it redelivers. There is
   one store instance shared by the write and read sides, injected by
   constructor — no global singletons that could diverge.
6. **One process, no CQRS.** Ingestion and queries are served by the
   same process over the same store. Writes are serialized with a lock
   per channel (never a global write lock); reads use `RLock`, so the
   default 3 concurrent clients — or more via `--concurrency-level` —
   only block each other when they hit the same rocket.
7. **Speed clamped at 0.** A decrease that would take speed below zero
   leaves it at zero. This is decision I took for simplicity reasons, not a requirement of the
   statement, but most likely it would require more thoughtful approach to match reality. 

## What was deliberately not done.

- **Persistence (Postgres/Redis).** The challenge is evaluated against a
  live run of the test binary, I decided not to go for a persistence implementation because I felt that would
be time-consuming and a risk in order to fulfill the test under the 6 hours. 
- **Kafka or any real broker.** The test program posts over HTTP; adding
  a broker would mean operating extra infrastructure for no change in
  the delivery semantics that matter here [imo we are talking about at-least-once, out of order].
- **Channel partitioning / sharding.** Per-channel locking already
  removes the contention a single instance can exploit; sharding across processes is something way out of the scope.

## Testing

```bash
go test ./...
```

The suite runs without network access and covers the behaviors the
delivery is graded on: out-of-order application, duplicate and conflict
handling, buffering before the launch, terminal explosion semantics,
concurrent channels, and list ordering. Run with `-race` to also check
the concurrency guarantees:

```bash
go test -race ./internal/...
```

## Project layout

```
.
├── main.go                 # Entry point (defaults: HTTP :8088, gRPC :8000)
├── Dockerfile              # Multi-stage build used by docker compose
├── docker-compose.yml      # Runs the service with ports published
├── Makefile                # proto / run / run-local / test / run-rockets
├── internal/
│   ├── adapters/           # gRPC + grpc-gateway adapters (HTTP mapping)
│   ├── application/        # RocketEventProcessor: write path and queries
│   ├── domain/
│   │   ├── messages/       # Event types, metadata, time parsing
│   │   └── rockets/        # Rocket entity + Store (per-channel inbox)
│   └── bootstrap.go        # Wiring: one store injected into both adapters
├── pkg/                    # App bootstrap and configuration helpers
├── proto/                  # API definitions (generated code is gitignored)
└── tools/proxy/            # Tiny debug proxy used during development
```
