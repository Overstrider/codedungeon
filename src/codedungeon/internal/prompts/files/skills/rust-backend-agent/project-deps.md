# project-deps.md

Detected deps for this repo. Scaffold — agent fills on first run from `Cargo.toml` + `crates/*/Cargo.toml`.

## Workspace / Crates
<!-- fill: list of workspace members + crate roots -->

## Runtime
- tokio: ?.?
- axum: ?.?

## DB
- sqlx: ?.? (features: runtime-tokio-rustls, postgres, uuid, chrono, macros)
- connection pool: ?

## Serialization
- serde: ?.?
- serde_json: ?.?

## Errors
- thiserror: ?.?
- anyhow: ?.? (if present)

## Observability
- tracing: ?.?
- tracing-subscriber: ?.?
- opentelemetry: ? (if present)

## Testing
- tokio (features = ["test-util"]): ?
- sqlx-cli: ?

## Notes
- Re-scan when `Cargo.toml` changes.
- If entry `?` remains, agent must scan + Write before proceeding.
