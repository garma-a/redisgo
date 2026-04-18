# AGENTS.md

## Fast Facts
- This is a single-module Go repo (`module github.com/GARMA-A/redisgo`) for the CodeCrafters Redis challenge.
- The real server entrypoint is `cmd/main.go` (the README still references `app/main.go`).
- Toolchain is pinned to Go 1.26 (`go.mod`, `codecrafters.yml`).

## Commands (source of truth: `Makefile`, `.codecrafters/*`)
- Run locally: `make run` (equivalent to `go run cmd/main.go`).
- Run with the local CodeCrafters wrapper: `./your_program.sh [args]` (builds `cmd/*.go` to `/tmp/codecrafters-build-redis-go`, then executes it).
- Build binary: `make build` (outputs `bin/redis-server`).
- Preferred full test pass: `go test ./...`.
- `make test` only runs `go test ./internal/...` (it skips `cmd`).
- Focused RESP test: `go test ./internal/resp -run TestParse`.

## Code Map
- TCP accept loop + per-connection goroutine: `cmd/main.go`.
- Command dispatch switch (`PING`, `ECHO`, `SET`, `GET`, `RPUSH`, `LRANGE`, `LPUSH`, `LLEN`, `LPOP`, `BLPOP`, `XADD`, `TYPE`): `internal/server/handler.go`.
- Command-specific handlers: `internal/server/*CommandHandler.go`.
- In-memory state and synchronization (`sync.RWMutex`, strings/lists/streams, TTL, BLPOP waiters): `internal/store/database.go`.
- RESP parser and parser tests: `internal/resp/parser.go`, `internal/resp/parser_test.go`.

## Repo-Specific Gotchas
- RESP framing is currently simplistic: `HandleClient` reads fixed 1024-byte chunks and calls `resp.Parse` per read. Protocol/framing work usually requires coordinated changes in both `internal/server/handler.go` and `internal/resp/parser.go`.
- `your_program.sh` is local-only; remote runs use `.codecrafters/compile.sh` + `.codecrafters/run.sh`. If build/run behavior changes, keep both paths aligned.
- `tmp/` is ignored and used by Air (`.air.toml` builds `./tmp/redisgo`); do not commit temp binaries.
- There is no repo CI workflow, lint config, or pre-commit config checked in; do not assume checks beyond Go build/test commands.
