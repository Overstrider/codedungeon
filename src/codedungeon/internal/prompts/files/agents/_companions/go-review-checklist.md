# go-review-checklist.md

Go review-mode concerns. Loldinis-sentinel-reviewer-go reads this on demand when scoping a finding.

## Types & Interfaces
- Exact structs: fields + types + JSON/DB tags.
- Interfaces small (1–3 methods). Accept interfaces, return structs.
- Functional options (`WithTimeout`, `WithLogger`) for constructors.
- Zero-value useful where possible.
- Named types for domain IDs (`type PropertyID string`), not raw primitives.

## Error Handling
- Every fallible fn returns `error` last.
- Wrap with context: `fmt.Errorf("doing X: %w", err)`. Every wrap names the op.
- Sentinels (`var ErrNotFound = errors.New(...)`) where callers use `errors.Is`.
- Custom error types (implement `error`) where callers use `errors.As`.
- Map variant → HTTP status at handler boundary.
- Never ignore errors. No `_` for errors in prod paths.
- `panic` only on unrecoverable startup errors. Never in handlers.

## Concurrency
- Every goroutine: explicit exit path (ctx cancel / channel close / done).
- `context.Context` as first param on all I/O or goroutine-spawning fns.
- Parallel pattern: `errgroup.Group` / bounded worker pool / `sync.WaitGroup`. Justify.
- Shared state: channel / `sync.Mutex` / `sync.RWMutex` / `sync.Map`. Justify.
- Channel buffering explicit. No unbounded buffers.
- Graceful shutdown: signal → ctx propagation → timeout-bounded drain.

## HTTP / API
- Per endpoint: method, path, handler fn, middleware chain.
- Req/res types: JSON tags + validation.
- Middleware order: logging, recovery, auth, rate-limit, req ID. Reference existing.
- Ctx flows from `http.Request` through all layers.
- Graceful shutdown: `http.Server.Shutdown(ctx)` with timeout.
- gRPC: proto service def + interceptors + error-code mapping.

## Database
- Approach: `database/sql` / `sqlx` / `pgx` / ORM. Follow codebase.
- New table/col: migration SQL.
- Pool config: `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime`.
- Tx boundaries explicit. Use `sql.Tx` or codebase helper.
- Always ctx in DB calls.
- Prepared / parameterized queries only. No concat.

## Package Structure
- Follow existing `internal/` / `cmd/` / `pkg/` layout.
- New pkg: single responsibility, name it, list exports.
- Avoid package-level state (global vars).
- Dep direction: handlers → service → repository. Never reverse.

## Testing
- Table-driven per fn with multi input/output.
- `t.Run` subtests.
- Handlers: `httptest.NewServer` / `httptest.NewRecorder`. Assert status, body, headers.
- Concurrent code: run with `-race`.
- Mocks: codebase-preferred (hand / mockgen / counterfeiter).
- Integration: testcontainers / test DB per existing pattern.

## Observability
- Structured log via `slog` (or codebase logger). List fields.
- OpenTelemetry: span names + attrs.
- Prometheus (if used): new counters / histograms.
- Always correlate with `request_id`.

## Performance
- Pre-alloc slices: `make([]T, 0, expected)`.
- `sync.Pool` only if profiling justifies.
- `strings.Builder` for loop concat.
- Streaming (`io.Reader`/`io.Writer`) for large payloads.
- `context.WithTimeout` on all external calls.
