# RedisGo Codebase Documentation

Part 1: The "One-Pager" High-Level Architecture

## Primary Purpose: What problem does this code solve?

This codebase implements a small in-memory Redis-compatible server for the CodeCrafters Redis challenge. It accepts TCP connections on port `6379`, parses RESP commands, routes them to command handlers, and stores data in memory across three logical types: strings, lists, and streams.

The implementation focuses on protocol and data-structure mechanics rather than production concerns (persistence, replication, clustering, advanced command compatibility). It is intentionally compact and challenge-oriented.

## Data Flow: How does data enter, move through, and exit this system?

1. A client connects over TCP and sends RESP-formatted bytes.
2. `cmd/main.go` accepts the socket and starts `server.HandleClient` in a goroutine.
3. `HandleClient` reads bytes into a fixed buffer, calls `resp.Parse`, and gets command tokens (`[]string`).
4. The command verb (`PING`, `SET`, `GET`, `RPUSH`, etc.) is uppercased and dispatched via `switch`.
5. Command-specific handlers validate arity/options and call methods on a shared `*store.DB`.
6. `store.DB` mutates or reads in-memory maps under a mutex (`sync.RWMutex`).
7. The handler writes a RESP response (`+`, `-`, `:`, `$`, `*`) back to the same socket.
8. For blocking list pops (`BLPOP`), request handlers register waiter channels and wait until a push wakes them or timeout expires.

## Entry Point: Where does the execution actually begin?

Execution begins in `cmd/main.go` at `func main()`. `main()` constructs the in-memory database, binds a TCP listener, and enters the accept loop. Each accepted connection is handled concurrently via `go server.HandleClient(conn, db)`.

---

Part 2: Deep-Dive Function Breakdown

## `cmd/main.go`

### Function Name & Signature: `main()`

What it does: Bootstraps the server process, creates shared state, listens on TCP port `6379`, and dispatches each client connection to a goroutine.

Step-by-Step Logic:
1. Calls `store.New()` to create one shared in-memory database.
2. Calls `net.Listen("tcp", "0.0.0.0:6379")` to open the Redis default port.
3. On bind error, prints a fixed message and exits with status code `1`.
4. Defers `l.Close()` to release the listener when process exits.
5. Logs startup text to stdout.
6. Enters an infinite accept loop.
7. For each accepted connection, spawns `go server.HandleClient(conn, db)`.

The "Why":
- A single `*store.DB` shared across connections is the simplest way to simulate Redis global keyspace semantics.
- Goroutine-per-connection is idiomatic Go and sufficient for this challenge scale.
- Explicit process exit on bind failure prevents a half-initialized server state.
- There is no shutdown orchestration/context wiring because the challenge does not require graceful lifecycle management.

## `internal/server/handler.go`

### Function Name & Signature: `HandleClient(conn net.Conn, db *store.DB)`

What it does: Implements the per-connection request loop: read bytes, parse RESP, route commands, and write RESP responses.

Step-by-Step Logic:
1. Defers `conn.Close()` to guarantee socket cleanup.
2. Allocates a `1024`-byte read buffer.
3. Loops forever calling `conn.Read(buf)`.
4. If read returns error:
   1. Ignores `io.EOF` as normal disconnect.
   2. Logs non-EOF read errors.
   3. Breaks loop.
5. Parses the received slice `buf[:n]` using `resp.Parse`.
6. On parse error, ignores the frame and continues.
7. Skips empty command arrays.
8. Uppercases the command name (`parts[0]`) for case-insensitive dispatch.
9. Runs a `switch` on command name.
10. For each known command:
    1. Validates minimum argument count.
    2. Writes `-ERR wrong number of arguments` when invalid.
    3. Calls the specialized handler (or inline implementation for `PING`/`ECHO`).
11. Unknown commands currently fall through silently (no default error branch).

The "Why":
- Centralized dispatch keeps protocol framing concerns in one place and data operation logic in specialized handlers.
- Arity checks are done before handler calls, so store methods can assume basic shape invariants.
- The fixed-size read and per-read parse are intentionally simple but not production-safe for RESP framing (partial/pipelined payloads can break parsing).
- Uppercasing command names mirrors Redis command-case flexibility.

## `internal/server/setCommandHandler.go`

### Function Name & Signature: `handleSet(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `SET key value [EX seconds | PX milliseconds]`, stores the string value, and returns `+OK`.

Step-by-Step Logic:
1. Reads `key := parts[1]` and `val := parts[2]`.
2. Initializes `expiry` as zero `time.Time` (means no TTL).
3. If at least five tokens are present, treats `parts[3]` as expiry option and `parts[4]` as numeric TTL.
4. Uppercases the option.
5. Parses TTL with `strconv.Atoi(parts[4])` (error is ignored).
6. If option is `EX`, computes `expiry = now + seconds`.
7. If option is `PX`, computes `expiry = now + milliseconds`.
8. Calls `db.Set(key, val, expiry)`.
9. Writes `+OK`.

The "Why":
- The handler keeps Redis option interpretation in protocol layer, while `store.DB` remains generic.
- Zero `time.Time` as sentinel avoids separate boolean flags for TTL existence.
- Ignoring `Atoi` errors is a simplification: malformed TTL effectively becomes `0`, which can expire immediately when checked later.

## `internal/server/getCommandHandler.go`

### Function Name & Signature: `handleGet(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `GET key`, reading a possibly expiring string and returning either RESP bulk string or null bulk string.

Step-by-Step Logic:
1. Calls `db.GetWithTTL(parts[1])`.
2. If key exists, writes `$<len>\r\n<value>\r\n`.
3. If key does not exist (or expired), writes `$-1\r\n`.

The "Why":
- TTL enforcement is delegated to `GetWithTTL`, so command handler remains protocol-only.
- Returning null bulk string aligns with Redis `GET` nil response semantics.

## `internal/server/rpushCommandHandler.go`

### Function Name & Signature: `handleRPush(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `RPUSH key value [value ...]`, appends one or many values, and returns integer result.

Step-by-Step Logic:
1. Declares `length`.
2. If multiple values provided (`len(parts) > 3`), calls `db.RPushMany(parts[1], parts[2:])`.
3. Else calls `db.RPush(parts[1], parts[2])`.
4. Writes integer response `:<length>\r\n`.

The "Why":
- Splitting single vs many avoids extra slice handling for the single-value hot path.
- Delegating list/waiter behavior to store keeps handler thin and testable.

### Function Name & Signature: `handleLRange(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `LRANGE key start stop` and returns a RESP array of list elements.

Step-by-Step Logic:
1. Parses `start` and `stop` as `int64` using `strconv.ParseInt` (errors ignored).
2. Calls `db.LRange(parts[1], int(start), int(stop))`.
3. Writes RESP array header with result size.
4. Iterates returned items and writes each as RESP bulk string.

The "Why":
- Index normalization logic is centralized in store to keep command layer minimal.
- Ignoring parse errors defaults invalid numbers to `0`, a simplification that may diverge from strict Redis error behavior.

### Function Name & Signature: `handleLPush(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `LPUSH key value [value ...]`, prepends values and returns integer result.

Step-by-Step Logic:
1. Declares `length`.
2. Uses `db.LPushMany` when multiple values are provided.
3. Uses `db.LPush` for single value.
4. Writes integer response.

The "Why":
- Mirrors `RPUSH` structure for consistency and maintainability.

### Function Name & Signature: `handleLLEN(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `LLEN key`, returning list length as RESP integer.

Step-by-Step Logic:
1. Calls `db.LLEN(parts[1])`.
2. Writes `:<len>\r\n`.

The "Why":
- Thin pass-through handler by design; all state logic belongs in `store.DB`.

### Function Name & Signature: `handleLPop(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `LPOP key [count]` for both single and multi-pop variants.

Step-by-Step Logic:
1. If count is present (`len(parts) > 2`), delegates to `handleLPopMany`.
2. Otherwise calls `db.LPop(parts[1])`.
3. If returned value is non-empty, writes it as bulk string.
4. If empty, writes null bulk string `$-1`.

The "Why":
- Wrapper pattern keeps single and multi-return RESP shapes separate and readable.
- Empty string is used as "not found" sentinel in `LPop`; this is simple but conflates empty string values with missing values.

### Function Name & Signature: `handleLPopMany(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles multi-pop branch of `LPOP`, returning an array of popped elements.

Step-by-Step Logic:
1. Parses `count` with `strconv.Atoi` (errors ignored).
2. Calls `db.LPopMany(parts[1], count)`.
3. Writes RESP array header.
4. Writes each popped value as bulk string.

The "Why":
- Keeps batch pop response formatting separate from single-pop null/bulk semantics.

### Function Name & Signature: `handleBLPOP(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `BLPOP key timeout`, either returning immediately, blocking until push, or timing out.

Step-by-Step Logic:
1. Extracts `key := parts[1]`.
2. Parses timeout as floating seconds via `strconv.ParseFloat`.
3. On parse error, writes `-ERR invalid timeout` and returns.
4. Creates an unbuffered `chan string`.
5. Calls `db.BLPOPWithOk(key, ch)`:
   1. If list has data, receives immediate element and returns it.
   2. If list is empty, DB registers `ch` as waiter and returns no value.
6. If timeout is positive:
   1. `select` waits on `ch` or `time.After(timeout)`.
   2. On value, writes two-element array `[key, value]`.
   3. On timeout, removes waiter via `db.RemoveWaiter` and writes null array `*-1`.
7. If timeout is `0`, blocks forever on `<-ch` and returns `[key, value]` when awakened.

The "Why":
- Channel-per-waiter models blocking consumer semantics cleanly without active polling.
- Immediate fast-path avoids unnecessary blocking setup when data already exists.
- Explicit timeout cleanup avoids leaking stale waiter channels.
- Using float timeout supports sub-second waits (Redis-compatible behavior).

### Function Name & Signature: `handleType(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `TYPE key`, returning `string`, `list`, `stream`, or `none`.

Step-by-Step Logic:
1. Reads key.
2. Calls `db.GetType(key)`.
3. Writes RESP simple string `+<type>\r\n`.

The "Why":
- Useful introspection command for challenge stages that add multiple data types.

## `internal/server/xaddCommandHandler.go`

### Function Name & Signature: `handleXAdd(conn net.Conn, db *store.DB, parts []string)`

What it does: Handles `XADD` stream insertion and maps store-layer validation errors to Redis-like protocol errors.

Step-by-Step Logic:
1. Calls `db.XAdd(parts[1], parts[2], parts[3:])` with stream key, entry id, and field/value pairs.
2. If error occurs, switches on specific sentinel errors:
   1. `ErrXAddIDTooSmall` -> monotonic-order violation error text.
   2. `ErrXAddIDZero` -> `0-0` forbidden error text.
   3. Default -> generic invalid stream id error text.
3. If success, returns the inserted id as RESP bulk string.

The "Why":
- Error mapping in handler keeps store package independent from Redis wire-message phrasing.
- Store returns typed errors, enabling precise protocol-level behavior.

## `internal/store/database.go` (Types / class-equivalents)

### Function Name & Signature: `type value struct { Data string; ExpiresAt time.Time }`

What it does: Represents one string key entry with optional expiration timestamp.

Step-by-Step Logic:
1. Stores payload in `Data`.
2. Stores TTL deadline in `ExpiresAt`; zero value means no expiry.

The "Why":
- Bundling value + expiry metadata in one struct makes TTL checks local and cheap.
- Using `time.Time` zero value as sentinel avoids extra flags.

### Function Name & Signature: `type list struct { values []string; waiters []chan string }`

What it does: Represents Redis-like list data plus blocked-consumer wait queues.

Step-by-Step Logic:
1. `values` stores list elements in order.
2. `waiters` stores channels for pending `BLPOP` operations.

The "Why":
- Co-locating data and waiter queues enables atomic push/pop/waiter decisions under one lock.

### Function Name & Signature: `type streamField struct { Name string; Value string }`

What it does: Represents a single stream field-value pair.

Step-by-Step Logic:
1. `Name` stores field key.
2. `Value` stores corresponding field value.

The "Why":
- Strongly typed pair avoids handling raw alternating slices once parsed.

### Function Name & Signature: `type streamEntry struct { ID string; Milliseconds uint64; Sequence uint64; Fields []streamField }`

What it does: Represents one stream record with both raw id and parsed numeric components.

Step-by-Step Logic:
1. `ID` preserves original `<ms>-<seq>` string.
2. `Milliseconds` and `Sequence` store parsed numeric values.
3. `Fields` stores field-value payload.

The "Why":
- Keeping parsed numeric parts avoids repeated parsing when comparing stream id order.

### Function Name & Signature: `type stream struct { entries []streamEntry }`

What it does: Holds ordered stream entries for a stream key.

Step-by-Step Logic:
1. Maintains append-only `entries` slice.

The "Why":
- Append-only ordering naturally matches stream id monotonic insertion.

### Function Name & Signature: `type DB struct { mu sync.RWMutex; data map[string]value; lists map[string]*list; streams map[string]*stream }`

What it does: Central in-memory keyspace object managing all data types and concurrency control.

Step-by-Step Logic:
1. `mu` coordinates safe concurrent access.
2. `data` stores string keys.
3. `lists` stores list keys.
4. `streams` stores stream keys.

The "Why":
- Separate maps by type avoid interface{} casting and simplify command-specific operations.
- Single lock is simple and consistent for challenge complexity.

### Function Name & Signature: `type testCase struct { name string; input []byte; want []string; wantErr bool }`

What it does: Defines one parser test vector in table-driven tests.

Step-by-Step Logic:
1. `name` labels subtest.
2. `input` is raw RESP bytes.
3. `want` is expected parsed output.
4. `wantErr` indicates error expectation.

The "Why":
- Standard table-driven shape keeps tests concise and easy to extend.

## `internal/store/database.go` (Functions and methods)

### Function Name & Signature: `newList() *list`

What it does: Allocates a new empty list container.

Step-by-Step Logic:
1. Creates `list` with `values: []string{}`.
2. Returns pointer.

The "Why":
- Explicit constructor keeps list initialization consistent in one place.

### Function Name & Signature: `(db *DB) getOrCreateList(key string) *list`

What it does: Returns existing list for key or creates/stores a new one.

Step-by-Step Logic:
1. Checks `db.lists[key]` and returns it if present and non-nil.
2. Otherwise creates a list via `newList()`.
3. Saves it to `db.lists[key]`.
4. Returns new list.

The "Why":
- Eliminates duplicate map-init logic across all list operations.
- Assumes caller already holds appropriate lock.

### Function Name & Signature: `newStream() *stream`

What it does: Allocates a new empty stream container.

Step-by-Step Logic:
1. Creates `stream` with empty `entries` slice.
2. Returns pointer.

The "Why":
- Mirrors list constructor for consistency and future extensibility.

### Function Name & Signature: `(db *DB) getOrCreateStream(key string) *stream`

What it does: Returns existing stream for key or creates/stores a new one.

Step-by-Step Logic:
1. Looks up `db.streams[key]`.
2. Returns existing stream when found and non-nil.
3. Otherwise creates a stream and inserts into map.
4. Returns the created stream.

The "Why":
- Centralized lazy initialization reduces repetitive nil checks.

### Function Name & Signature: `(lst *list) isSendToWaiter(value string) bool`

What it does: Delivers a pushed list value directly to the oldest blocked waiter if any exist.

Step-by-Step Logic:
1. If `waiters` is empty, returns `false`.
2. Takes first waiter channel (`FIFO`).
3. Sets first slot to `nil` and removes it from slice.
4. Sends `value` to that channel.
5. Returns `true`.

The "Why":
- Encapsulates wake-up mechanics to keep push methods concise.
- FIFO order approximates fair waiter servicing.
- Setting the slot to `nil` before slicing can help GC by dropping references sooner.

### Function Name & Signature: `New() *DB`

What it does: Constructs an initialized database instance.

Step-by-Step Logic:
1. Allocates `DB`.
2. Initializes `data`, `lists`, and `streams` maps.
3. Returns pointer.

The "Why":
- Prevents nil-map panics and standardizes DB creation path.

### Function Name & Signature: `(db *DB) Set(key, val string, expiry time.Time)`

What it does: Inserts or updates a string key and optional expiration timestamp.

Step-by-Step Logic:
1. Acquires write lock.
2. Assigns `db.data[key] = value{Data: val, ExpiresAt: expiry}`.
3. Releases lock.

The "Why":
- Atomic replacement semantics align with Redis `SET` overwrite behavior.

### Function Name & Signature: `(db *DB) Get(key string) (string, bool)`

What it does: Retrieves raw string key without TTL expiration cleanup.

Step-by-Step Logic:
1. Acquires read lock.
2. Looks up map entry.
3. Returns `val.Data` and existence flag.
4. Releases lock via `defer`.

The "Why":
- Useful primitive when caller wants raw lookup behavior.
- Separate method avoids coupling all reads to TTL checks.

### Function Name & Signature: `(db *DB) GetWithTTL(key string) (string, bool)`

What it does: Retrieves key while lazily enforcing expiration.

Step-by-Step Logic:
1. Acquires read lock and fetches value.
2. Releases read lock.
3. If key exists and has non-zero expiry and `now > ExpiresAt`:
   1. Acquires write lock.
   2. Deletes key.
   3. Releases write lock.
   4. Marks as non-existent.
4. Returns data if still valid; otherwise returns `"", false`.

The "Why":
- Lazy expiration avoids background sweeper complexity.
- Read-then-upgrade locking minimizes write-lock duration for non-expired keys.

### Function Name & Signature: `(db *DB) RPush(key string, value string) int`

What it does: Appends one value to list tail, or wakes a blocked waiter directly.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. Captures current length.
4. Tries `lst.isSendToWaiter(value)`:
   1. If delivered to waiter, returns `currentLen + 1`.
5. Otherwise appends value to `lst.values`.
6. Returns resulting length.

The "Why":
- Direct handoff avoids storing data when a `BLPOP` is waiting.
- Return value follows command semantics focused on pushed element count, not necessarily post-wakeup storage length.

### Function Name & Signature: `(db *DB) RPushMany(key string, values []string) int`

What it does: Appends multiple values at tail, delivering to waiters first when present.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. Stores `currentLen`.
4. Iterates each value:
   1. If waiter exists, sends directly and continues.
   2. Else appends to list.
5. Returns `currentLen + len(values)`.

The "Why":
- Batch path reduces handler overhead for variadic pushes.
- Return expression matches "number pushed" interpretation used in this implementation.

### Function Name & Signature: `parseExplicitStreamID(id string) (uint64, uint64, error)`

What it does: Parses stream id text `<milliseconds>-<sequence>` into numeric components.

Step-by-Step Logic:
1. Splits input by `"-"`.
2. Validates exactly two parts.
3. Parses first part as unsigned integer milliseconds.
4. Parses second part as unsigned integer sequence.
5. Returns parsed values or `ErrXAddInvalidID`.

The "Why":
- Isolates validation/parsing from `XAdd` to keep method focused on insertion rules.
- Unsigned parsing enforces non-negative id components.

### Function Name & Signature: `isIDStrictlyGreater(milliseconds, sequence, lastMilliseconds, lastSequence uint64) bool`

What it does: Compares two stream ids lexicographically by `(milliseconds, sequence)`.

Step-by-Step Logic:
1. If new milliseconds greater than last, returns `true`.
2. If smaller, returns `false`.
3. If equal milliseconds, returns whether new sequence is greater.

The "Why":
- Encodes Redis stream monotonicity rule cleanly and testably.
- Avoids string-based comparisons that could misorder numeric values.

### Function Name & Signature: `(db *DB) XAdd(key, id string, fields []string) (string, error)`

What it does: Adds a stream entry after validating id syntax and ordering constraints.

Step-by-Step Logic:
1. Acquires write lock.
2. Parses `id` using `parseExplicitStreamID`.
3. Rejects `0-0` with `ErrXAddIDZero`.
4. If stream already has entries:
   1. Reads last entry.
   2. Ensures new id is strictly greater via `isIDStrictlyGreater`.
   3. Rejects otherwise with `ErrXAddIDTooSmall`.
5. Retrieves/creates target stream.
6. Converts alternating string slice `fields` into typed `[]streamField` in steps of two.
7. Appends new `streamEntry` with raw and numeric id plus fields.
8. Returns id.

The "Why":
- Write lock guarantees ordering checks and append happen atomically.
- Storing numeric id components once avoids reparsing later.
- Assumes even field count because command handler already enforces arity.

### Function Name & Signature: `(db *DB) LRange(key string, start, stop int) []string`

What it does: Returns a sub-slice of list values using Redis-like negative index behavior.

Step-by-Step Logic:
1. Acquires read lock and reads list pointer, then unlocks.
2. If list missing or empty, returns empty slice.
3. Normalizes `start`:
   1. If negative, offsets from end.
   2. Clamps to `0` minimum.
4. Normalizes `stop` similarly.
5. Clamps `stop` to last valid index.
6. If `start > stop`, returns empty slice.
7. Returns `list[start : stop+1]`.

The "Why":
- Implements core Redis index semantics with minimal branching.
- Returning a slice view is memory-efficient, though it shares backing array with internal state.

### Function Name & Signature: `(db *DB) LPush(key string, value string) int`

What it does: Prepends one value to list head, or hands it directly to a waiting blocker.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. Captures current length.
4. Attempts waiter handoff via `isSendToWaiter`.
5. If no waiter, prepends with `append([]string{value}, lst.values...)`.
6. Returns new logical length.

The "Why":
- Head insertion keeps semantics correct for `LPUSH`.
- Prepend by slice rebuild is simple and fine for small challenge workloads.

### Function Name & Signature: `(db *DB) LPushMany(key string, values []string) int`

What it does: Prepends multiple values in sequence, preserving Redis `LPUSH` ordering behavior.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. Stores `currentLen`.
4. Iterates each input value in order:
   1. If waiter exists, sends directly.
   2. Else prepends value.
5. Returns `currentLen + len(values)`.

The "Why":
- Repeated prepend means later arguments end up closer to head, matching Redis `LPUSH key a b` -> `[b, a, ...]`.

### Function Name & Signature: `(db *DB) LPop(key string) string`

What it does: Pops one value from list head and returns empty string when unavailable.

Step-by-Step Logic:
1. Acquires write lock.
2. Initializes `val := ""`.
3. If list exists and non-empty:
   1. Reads first element.
   2. Re-slices list to remove head.
4. Returns `val`.

The "Why":
- Minimal API for handlers that already treat empty-string as null response.
- Simplicity comes with ambiguity for legitimate empty string elements.

### Function Name & Signature: `(db *DB) LPopWithOK(key string) (string, bool)`

What it does: Pops one value with explicit success flag.

Step-by-Step Logic:
1. Acquires write lock.
2. If list has data, pops head and returns `(value, true)`.
3. Else returns `("", false)`.

The "Why":
- Avoids empty-string ambiguity when callers need exact existence information.

### Function Name & Signature: `(db *DB) BLPOPWithOk(key string, ch chan string) (string, bool)`

What it does: Atomic check/register operation for blocking list pop behavior.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. If list is empty:
   1. Appends waiter channel to `lst.waiters`.
   2. Returns `("", false)`.
4. If list has values:
   1. Pops head.
   2. Returns `(value, true)`.

The "Why":
- Prevents race where data arrives between "check empty" and "register waiter" steps.
- This is the key atomic primitive that makes `BLPOP` reliable.

### Function Name & Signature: `(db *DB) RemoveWaiter(key string, ch chan string)`

What it does: Removes a specific waiting channel from a list's waiter queue.

Step-by-Step Logic:
1. Acquires write lock.
2. Looks up target list.
3. If list missing, returns.
4. Scans waiters for channel identity match.
5. Removes first match using slice splice.
6. Returns.

The "Why":
- Required for timeout cleanup in `BLPOP` to avoid stale queue entries.
- Identity comparison on channel values is precise and efficient.

### Function Name & Signature: `(db *DB) LLEN(key string) int`

What it does: Returns list length or zero when list does not exist.

Step-by-Step Logic:
1. Acquires read lock.
2. If list exists and non-nil, returns `len(lst.values)`.
3. Else returns `0`.

The "Why":
- Mirrors Redis behavior (`LLEN` of missing key is `0`).

### Function Name & Signature: `(db *DB) LPopMany(key string, count int) []string`

What it does: Pops up to `count` elements from list head and returns them as a slice.

Step-by-Step Logic:
1. Acquires write lock.
2. If list exists and non-empty:
   1. Clamps `count` to list length.
   2. Takes prefix slice `vals := lst.values[:count]`.
   3. Advances list head by `count`.
   4. Returns popped values.
3. If empty/missing, returns empty slice.

The "Why":
- Batch removal is more efficient than repeated single pops.

### Function Name & Signature: `(db *DB) LPopWaiter(key string)`

What it does: Removes the first waiter entry from a list.

Step-by-Step Logic:
1. Acquires write lock.
2. Retrieves/creates list.
3. Re-slices `waiters` to drop index `0`.

The "Why":
- Intended as a queue-pop helper for waiters.
- In current code it is unused and lacks an empty-check guard, so calling it on an empty waiter list would panic.

### Function Name & Signature: `(db *DB) GetType(key string) string`

What it does: Returns type label for key by checking stream/list/string maps.

Step-by-Step Logic:
1. Acquires read lock.
2. If key exists in streams map, returns `"stream"`.
3. Else if exists in lists map, returns `"list"`.
4. Else if exists in data map, returns `"string"`.
5. Else returns `"none"`.

The "Why":
- Separate type maps make type checks trivial.
- Priority order resolves collisions if the same key appears in multiple maps.

## `internal/resp/parser.go`

### Function Name & Signature: `Parse(data []byte) ([]string, error)`

What it does: Parses RESP array-of-bulk-strings payloads into command token slices.

Step-by-Step Logic:
1. Rejects empty input.
2. Converts bytes to string and splits by `"\r\n"`.
3. Validates first token exists.
4. Ensures first token starts with `*` (array frame).
5. Parses array length from token after `*`.
6. Initializes result slice and index pointer.
7. Loops exactly `numElements` times using Go integer range.
8. For each element:
   1. Validates a token exists and starts with `$`.
   2. Parses declared bulk length.
   3. Moves to next token for data payload.
   4. Validates actual payload length equals declared length.
   5. Appends payload string.
9. Returns parsed token slice.
10. Returns unsupported-format error when payload is not array-based RESP.

The "Why":
- Strict token-by-token checks catch malformed/incomplete frames early.
- Returning plain `[]string` keeps downstream command handlers simple.
- The parser intentionally supports only the subset needed for command arrays, not the full RESP type system.
- The `for range numElements` loop uses modern Go syntax (valid in Go 1.22+).

## `internal/resp/parser_test.go`

### Function Name & Signature: `TestParse(t *testing.T)`

What it does: Validates parser behavior across valid and invalid RESP scenarios via table-driven subtests.

Step-by-Step Logic:
1. Iterates predefined `tests` slice.
2. Runs each case using `t.Run(tc.name, ...)`.
3. Calls `Parse(tc.input)`.
4. Checks whether error presence matches `tc.wantErr`.
5. If error expectation passes, compares parsed result with `tc.want` using `reflect.DeepEqual`.
6. Emits test failures with informative output for mismatch.

The "Why":
- Table-driven tests are idiomatic Go for parser matrices.
- Named subtests make failing scenarios easy to identify.

---

Part 3: State & Dependencies

## How state is managed across these functions

1. Global runtime state is a single shared `*store.DB` allocated once in `main()` and passed to every connection handler.
2. `DB` stores state by data type in separate maps:
   1. `data` for string values (`SET`/`GET`).
   2. `lists` for list values and BLPOP waiter queues.
   3. `streams` for stream entries (`XADD`).
3. Concurrency is controlled by one `sync.RWMutex`:
   1. Read-only paths (`Get`, `LLEN`, `GetType`) use `RLock`.
   2. Mutating paths (`Set`, push/pop, XADD, waiter registration/removal) use `Lock`.
4. TTL state is stored per-string key in `ExpiresAt`; expiration is lazy and checked on reads (`GetWithTTL`).
5. Blocking pop state uses channels:
   1. Empty-list `BLPOP` registers a waiter channel.
   2. Later `LPUSH`/`RPUSH` may send directly to that waiter instead of storing value.
   3. Timeouts remove waiter channels explicitly.
6. Stream state uses append-only entries and explicit monotonic id validation (`milliseconds-sequence`).
7. No persistence exists: all state is in-memory and process-local, so restart loses data.

## Crucial external dependencies and libraries

This project has no third-party libraries in active use; it relies on Go standard library and internal packages.

### Internal packages

1. `github.com/GARMA-A/redisgo/internal/server`
   - Connection loop command dispatch and RESP response formatting.
2. `github.com/GARMA-A/redisgo/internal/store`
   - In-memory data model, synchronization, list blocking mechanics, stream validation.
3. `github.com/GARMA-A/redisgo/internal/resp`
   - RESP command parsing.

### Standard library packages

1. `net`
   - TCP listener, accepted connections, read/write socket API.
2. `io`
   - `io.EOF` handling for disconnect detection.
3. `sync`
   - `sync.RWMutex` for concurrent state access.
4. `time`
   - TTL math and BLPOP timeout scheduling.
5. `strconv`
   - Numeric parsing for command options/indexes/ids.
6. `strings`
   - Command normalization and RESP token parsing helpers.
7. `fmt`
   - RESP string formatting and log output.
8. `errors`
   - Sentinel error values for stream validation.
9. `os`
   - Process exit on startup bind failure.
10. `testing`, `reflect`
   - Unit tests for parser behavior.

## Important design implications

1. RESP framing is simplistic: `HandleClient` parses each read buffer independently, so fragmented or pipelined commands may not be handled robustly.
2. Key types are stored in separate maps without cross-type exclusivity enforcement; a key could exist in multiple maps unless command usage avoids that.
3. Some numeric parse errors are ignored (`Atoi`/`ParseInt` in handlers), which can silently coerce invalid client input.
4. List prepend operations (`LPUSH`) are O(n) due to slice rebuilding, acceptable for challenge scope.
5. `LPopWaiter` is currently unused and would panic on empty waiter slices.
