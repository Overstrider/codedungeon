# guardrails.md (rust-backend subset of task.md §5)

Read only the section matching THIS change.

## Performance
- List endpoints: paginate (cursor/keyset > offset on large tables).
- No N+1: plan eager load or batched query for related data.
- Long ops → async/background (queue, tokio task). Never block req.
- New queries on large tables: plan supporting index. Specify cols.
- Respect pool config. No ad-hoc conns outside pool.
- Cache-worthy reads → existing cache layer if present.

## Concurrency
- Transactions: acquire locks in consistent order. Specify order on multi-table.
- Keep tx short — only atomic work inside.
- Parallel work: `tokio::join!` / `JoinSet`. Justify fixed vs dynamic N.
- Cancel safety in `tokio::select!` branches.

## Rate Limiting
- New public endpoint → apply existing rate limiter middleware.
- If no limiter exists → prerequisite, not assumption.

## Security
- Validate + sanitize at boundary (handler). Existing validator crate.
- Parameterized queries only (`sqlx::query!`). No string concat.
- New endpoint goes through auth/authz middleware — do not skip or dup.
- Never log secrets (tokens, passwords, PII). Structured log with correlation IDs.
- Least-privilege on new DB roles.

## Data Integrity
- Constraints: unique / FK / not-null for every new col/table.
- Migrations backwards-compat on rolling deploy.
- Idempotency: specify where ops must be idempotent (retries).

## Errors
- Reuse existing error types. No new response formats.
- Map variant → HTTP status at handler boundary.
- Every public fn: clearly defined input/output/error types.

## API
- Consistent shape: existing `ApiResponse<T>` / `ApiError` wrapper.
- Versioned path: `/api/v{N}/...` matches existing.
- Content-Type: application/json unless binary.

## Observability
- Entry/exit/error structured logs per endpoint (existing pattern).
- Tracing spans per op if codebase uses OpenTelemetry.
- Health checks unaffected.
