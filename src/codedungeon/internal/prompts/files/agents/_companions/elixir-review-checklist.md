# elixir-review-checklist.md

Elixir review concerns. elixir-reviewer reads on demand.

## Modules & Types
- Full namespace per module. Exported fns + `@spec`.
- Structs: fields + types + `enforce_keys` where needed.
- `@type` / `@spec` on public fns.
- Domain IDs: typed structs / dedicated modules.

## Supervision & Process Architecture
- Every new process (GenServer / Task / Agent): position in supervision tree + strategy (`one_for_one` / `one_for_all` / `rest_for_one`).
- `child_spec`: restart (`:permanent` / `:temporary` / `:transient`) + shutdown timeout.
- DynamicSupervisor: start/stop contract.
- GenServer state shape + `init/1` initial state.
- Pick: GenServer (stateful long-lived) / Agent (simple state) / Task (one-off async) / Oban (bg jobs) / Broadway/GenStage (streaming).
- Every `handle_call` / `handle_cast` / `handle_info`: message pattern + return tuple.

## Error Handling
- "Let it crash" under supervision. Name the strategy that recovers.
- Tagged tuples (`{:ok, v}` / `{:error, r}`) at context boundaries.
- `with` for happy-path chaining + `else` for error mapping.
- Rescue only at system boundaries (HTTP handler / external).
- Phoenix: map error tuples → HTTP response / socket assigns.

## Phoenix & LiveView
- Which context module owns each op.
- Per endpoint: controller/LiveView module + action/event + route + plugs/middleware.
- LiveView: `mount/3` assigns + `handle_event/3` per action + `handle_info/2` for PubSub.
- LiveView state: full assigns map + types. Which assigns trigger re-render.
- Real-time: PubSub topic + broadcast msg + subscribers.
- LiveComponent: when to extract + assigns.
- Streams for large collections: `stream/3` + `stream_insert/3`.

## Ecto
- New schema: fields + types + assocs + required fields per changeset.
- Changeset validations per fn.
- Queries: schema-based vs schemaless `from`. Follow existing.
- Multi-step atomic: `Ecto.Multi` with named steps.
- Migration: col types + constraints + indexes + reversibility.
- Preload: `Ecto.Query.preload` in query (avoid N+1).

## Concurrency
- Parallel: `Task.async_stream` (`max_concurrency`, `ordered`) or `Task.Supervisor`.
- Backpressure: GenStage/Broadway or GenServer-based.
- Periodic: `:timer.send_interval` or Oban scheduled.
- PubSub: topic convention + publisher + subscribers.
- ETS: only for read-heavy shared state. Name + access type + concurrency opts.

## Testing
- ExUnit: module + `use` decl (`DataCase` / `ConnCase` / `FeatureCase`) + describe.
- Per test: scenario + assertion. Table-style for multi.
- Ecto sandbox: mode + setup.
- Mox: behaviors + expectations.
- LiveView: `live/2` + `render_submit` / `render_click` + HTML assertions.
- GenServer: `start_supervised` + message assertions.
- Async: `assert_receive` + timeout.

## Observability
- Telemetry events: `[:app, :context, :action]` convention + measurements + metadata.
- Logger: structured `Logger.metadata` (request_id, user_id).
- LiveDashboard: new metrics visible.

## Performance
- Avoid single-GenServer bottlenecks → pool or ETS.
- LiveView large payload: `temporary_assigns`.
- Large binaries: `:binary.copy/1` to drop parent ref.
- Idle GenServers: `hibernate`.
