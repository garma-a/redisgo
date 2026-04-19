# RedisGo

RedisGo is a lightweight Redis-compatible in-memory database server implemented in Go. It accepts TCP clients on port `6379`, parses RESP arrays, and executes a focused set of Redis-style commands.

## Features

- TCP server compatible with Redis client connections
- RESP array parsing (`*...` / `$...`)
- In-memory key-value storage with concurrency safety
- Core command support: `PING`, `ECHO`, `SET`, `GET`
- `SET` supports optional expiry: `EX` (seconds) and `PX` (milliseconds)
- TTL expiration handling on read

## Backend Engineering Notes

- RESP parsing is isolated in `internal/resp` for protocol-layer separation.
- Connection handling and command dispatch live in `internal/server`.
- Storage is guarded by `sync.RWMutex` to support concurrent clients.
- Expired keys are lazily cleaned during read access.

## Architecture

```text
cmd/redis-server/main.go      # Server bootstrap and listener
internal/server/handler.go    # Command dispatch and RESP responses
internal/resp/parser.go       # RESP parser
internal/store/database.go    # Thread-safe in-memory store
```

## Prerequisites

- Go 1.24+

## Run

```bash
make run
```

Server starts on:

```text
0.0.0.0:6379
```

## Build

```bash
make build
```

Binary output:

```text
bin/redis-server
```

## Test

```bash
make test
```

## Manual Verification with `redis-cli`

In one terminal:

```bash
make run
```

In another terminal:

```bash
redis-cli -p 6379
PING
ECHO "hello"
SET sample 42
GET sample
SET temp value EX 2
GET temp
```

After two seconds, `GET temp` should return nil.

## Notes

- The project intentionally focuses on a minimal command surface for educational clarity.
- Commands outside the implemented set are not handled.

## Future Enhancements

- Add more Redis commands (e.g., `DEL`, `EXISTS`, `INCR`).
- Improve protocol-level error responses for unsupported commands.
- Add integration tests using a real Redis client against the running server.
