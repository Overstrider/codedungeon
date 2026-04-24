# python-review-checklist.md

Python review-mode concerns. python-reviewer reads on demand.

## Types & Data Modeling
- Exact classes / dataclasses / Pydantic models. Name + all fields typed.
- Type hints mandatory on public signatures. `Optional` / `Union` / `Literal` / `TypeAlias` as needed.
- Pydantic: validators + `Field()` metadata + `model_config`.
- ORM models: columns + types + relationships + constraints + nullable.
- Dataclasses = internal; Pydantic = validation boundary; plain classes = services/repos.
- `Enum`/`StrEnum` for fixed sets; `Literal` for narrow string unions.

## Error Handling
- Custom exception hierarchy per codebase pattern. Name class + base.
- Specific exceptions over generic `Exception`/`ValueError`.
- API boundary handlers map domain exceptions → HTTP status + response shape.
- Never bare `except:`. No `except Exception:` unless re-raising.
- `raise ... from err` to preserve chain.
- Graceful-fail: `None` (with `Optional`) OR raise — follow codebase.

## Async & Concurrency
- Per fn: `async def` vs `def`. I/O → async in async-first codebase.
- `await` for I/O; `asyncio.gather` for parallel I/O; `asyncio.TaskGroup` (3.11+) for structured concurrency.
- CPU-bound: `asyncio.to_thread()` / `ProcessPoolExecutor`. Never block event loop.
- Background: codebase-standard (Celery / ARQ / FastAPI BackgroundTasks / asyncio tasks).
- `async with` for async ctx managers (sessions, HTTP clients).
- Sync Django: thread safety + `select_for_update()` / `transaction.atomic()`.

## Web Framework
- FastAPI: decorator + path + `response_model` + `status_code` + `Depends` + typed params (Path/Query/Body). OpenAPI summary/desc.
- Django: view (fn / class) + URL + serializer (DRF) / form + permissions + queryset.
- Middleware: position in stack + reference existing.
- DI (FastAPI `Depends`): fn + what it provides + lifetime (per-request / singleton).
- Auth: follow existing pattern. Name protected endpoints + permission check.

## Database
- ORM approach: SQLAlchemy (async/sync) / Django / raw SQL. Follow codebase.
- New tables: model class + cols (`Column` / `mapped_column`) + constraints + indexes + relationships.
- Migrations: Alembic rev message or Django migration. Types + defaults + nullable + indexes.
- Queries: ORM / raw / builder. Complex → describe SQL semantics (JOIN / subquery / aggregate).
- Tx: `async with session.begin():` (SQLAlchemy async) or `transaction.atomic()` (Django).
- Pool: reference existing. No new engines outside pattern.
- N+1: `selectinload`/`joinedload` (SA) or `select_related`/`prefetch_related` (Django).

## Package & Project
- Follow existing package layout + module placement.
- New deps: package + version constraint + correct section in `pyproject.toml` (prod vs dev). Confirm no duplicate.
- DI container: follow patterns for new services.
- `__init__.py` exports if codebase uses explicit public API.

## Testing
- Framework: pytest / Django TestCase / unittest. Follow existing.
- Per component: test file location + fn names + assertions.
- Fixtures: pytest fixtures for data / DB sessions / API clients. Reference existing.
- API: `TestClient` (FastAPI) / `APIClient` (DRF). Body + expected status + response assertions.
- Async tests: `@pytest.mark.anyio` / `@pytest.mark.asyncio`.
- Mocking: `unittest.mock.patch` target or `pytest-mock`. Exact patch path.
- DB tests: fixture isolation (tx / test DB / factory_boy).

## Observability
- Logging: `structlog` or `logging` per codebase. Logger name + level + structured fields.
- OpenTelemetry: span names + attrs.
- Prometheus: new counters / histograms.

## Performance
- List ops: comprehensions / generators over loops.
- Large data: generators / iterators with `yield`.
- Repeated expensive: `@functools.lru_cache` / `@functools.cache` with maxsize.
- Hot async paths: minimize `await`, batch DB queries, use pool.
- `__slots__` on data-heavy classes if codebase uses them.
