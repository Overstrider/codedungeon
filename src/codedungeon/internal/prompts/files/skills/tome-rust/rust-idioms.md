# rust-idioms.md

Rust-specific enrichment guidance. Read on demand — only the section matching THIS change. Ported from rust-specialist (Plan mode content).

## Types & Data Modeling
- Define exact structs/enums. Name them, list fields + types.
- DB row struct: ORM model + derive macros (`sqlx::FromRow`, `Deserialize`).
- API req/res struct: serde attrs (`rename_all = "camelCase"`, `deny_unknown_fields`, `skip_serializing_if`).
- Newtypes for domain IDs: `PropertyId(Uuid)` not raw `Uuid`.
- `Cow<'_, str>`, `Arc`, `Box` only where ownership efficiency demands.
- `#[derive(Debug, Clone, Serialize, Deserialize)]` minimal set — no blanket derives.

## Errors
- `thiserror` for library-style errors with structured variants (`#[error("...")]`).
- `anyhow` only in top-level app glue where context > match.
- Every module's error enum: name, variants, what each wraps.
- Handler boundary: map error variant → HTTP status (404/400/409/500). Specify mapping.
- Never plan `.unwrap()` / `.expect()` in prod paths. Flag existing unwraps.

## Async & Concurrency
- `async fn` vs sync: pick per operation. Runtime = tokio.
- CPU-bound inside async → `spawn_blocking`.
- Parallel: `tokio::join!` (fixed N) / `JoinSet` (dynamic N) / sequential awaits. Justify.
- Shared state primitive: `Arc<Mutex<T>>` / `Arc<RwLock<T>>` / `DashMap` / channels. Justify choice.
- Background tasks: reference existing codebase pattern (tokio tasks, job queue).
- Cancellation safety: note where `tokio::select!` branches must be cancel-safe.

## Database (sqlx)
- Query approach: raw SQL with `sqlx::query!` (compile-time checked) / `query_as!` / query builder.
- New tables/columns: migration SQL with types, constraints, indexes.
- Fetch: `fetch_one` (must exist), `fetch_optional` (may exist), `fetch_all` (list).
- Transaction boundary: specify which ops atomic. Acquire locks in consistent order — state order explicitly if multi-table.
- Connection pool: reference `sqlx::PgPool` / `deadpool` / `bb8` existing instance. No ad-hoc pools.
- Prepared statements implicit via `query!` macros — do not concat SQL.

## API Layer (axum)
- Per endpoint: HTTP method, path, extractor types (`Path`, `Query`, `Json`, `Extension`, custom), response type.
- Middleware stack: auth, rate limit, request ID, tracing span. Reference existing layer.
- Handler signature precise: `async fn handler(auth: AuthUser, Path(id): Path<Uuid>, Json(body): Json<CreateRequest>) -> Result<Json<Response>, AppError>`.
- Validation location: serde deserialize / dedicated validator crate / domain layer. Pick one, state where.
- `IntoResponse` impl for response types with non-200 status variants.

## Testing
- Unit tests: domain logic, pure fns. `#[test]`.
- Integration: handlers with test DB. `#[tokio::test]`. Reference existing fixtures.
- Assert: status codes, response body shape, DB state post-call, error variant.
- sqlx test: `#[sqlx::test]` macro if codebase uses it.
- Property tests: `proptest` / `quickcheck` only where invariants matter.

## Crates
- New dep: name + version. Justify why. Confirm no existing dep covers it.
- If codebase already has a crate for the purpose, use it. No alternatives.
- Pin major versions in `Cargo.toml`; minor/patch via `^`.
- Check `cargo audit` output for advisories before adding.

## Output subsection template (per change)

```
#### rust
- structs: [PropertyId(Uuid), CreatePropertyReq { title: String, price: i64, city: String }]
- errors: [CreateError::{Validation, Duplicate, Db(sqlx::Error)}]
- crates: [n/a | validator 0.18 — input validation]
- async: [single query per call, no spawn needed]
- db: [INSERT into properties via query_as!; returning id]
- api: [POST /api/v1/properties, Json<CreateRequest> → Json<Response> | AppError]
- error-mapping: [Validation → 400, Duplicate → 409, Db → 500]
- tests: [integration with test pool, assert 201 + DB row exists]
- references: [crates/api/src/users/create.rs]
```

Empty subsections = noise. Omit.
