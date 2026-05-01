# QA / Testing / Verification Module Design

Date: 2026-05-01
Status: design approved for Option 1, pending implementation plan

## Summary

Build `internal/qa` as an evidence-first verification module. The module must work in two entrypoints:

1. Standalone QA: a user can run `codedungeon qa ...` directly in a repo without starting the full codedungeon workflow.
2. Workflow QA: `codedungeon run` / full-cycle execution calls QA automatically as the verification phase, without requiring an agent to decide that QA should happen.

The core principle is that QA produces evidence, not opinions. Agents may propose plans, generate tests, or triage failures, but a PASS status is granted only by deterministic checks with captured artifacts.

This explicitly supports the `cli-tool-full-cycle` case: the CLI runner can complete the implementation workflow, enter QA, enforce the gate, and continue or stop based on QA result without an agent manually invoking QA.

## Current State

Existing CLI surface:

- `codedungeon qa run`
- `codedungeon qa record`
- `codedungeon qa secret-scan`
- `codedungeon qa validate-api`
- `codedungeon qa detect-framework`

Current behavior is useful but helper-shaped. It records command evidence into `verification_records`, can validate APIs, can scan secrets, and can detect framework commands. It does not yet have:

- A first-class QA session.
- A request/result contract.
- A durable execution artifact directory.
- A test plan model.
- Dependency preflight.
- Structured failure analysis.
- Structured findings and fix tasks.
- Playwright artifact capture.
- A standalone mode that can create its own QA run context.
- A full-cycle mode where the workflow invokes QA deterministically.

## Reference Lessons

The design follows these lessons from current agent QA and coding-agent evaluation work:

- SWE-bench: real software verification should be grounded in executable tests, not model self-assessment. Source: https://arxiv.org/abs/2310.06770
- SWE-bench Verified audit: bad tests, underspecified tasks, contamination, and environment drift can invalidate a pass/fail signal. Source: https://openai.com/index/why-we-no-longer-evaluate-swe-bench-verified/
- SWE-bench Pro: modern agent QA needs longer-horizon, multi-file, reproducible tasks and reliable environments. Source: https://labs.scale.com/papers/swe_bench_pro
- SWE-agent: agent-computer interfaces matter; tools should expose concise structured actions and observations. Source: https://arxiv.org/abs/2405.15793
- Agentless: simple localization, repair, and patch-validation workflows can outperform complex autonomous loops. Source: https://arxiv.org/abs/2407.01489
- AutoCodeRover: structured code search, test feedback, and fault localization improve autonomous repair. Source: https://arxiv.org/abs/2404.05427
- Reflexion, Self-Refine, Self-Debugging, and CodeT: execution feedback is useful for agent improvement, but it must be bounded and tied to real evidence. Sources:
  - https://arxiv.org/abs/2303.11366
  - https://arxiv.org/abs/2303.17651
  - https://arxiv.org/abs/2304.05128
  - https://arxiv.org/abs/2207.10397
- Playwright best practices: E2E tests should use user-visible behavior, resilient locators, web-first assertions, traces, reporters, and isolated auth state. Sources:
  - https://playwright.dev/docs/best-practices
  - https://playwright.dev/docs/locators
  - https://playwright.dev/docs/test-assertions
  - https://playwright.dev/docs/trace-viewer-intro
  - https://playwright.dev/docs/test-reporters
  - https://playwright.dev/docs/auth
  - https://playwright.dev/docs/ci
  - https://playwright.dev/docs/getting-started-mcp
  - https://playwright.dev/docs/test-agents

## Goals

- Provide a first-class `internal/qa` module with stable Go APIs.
- Support standalone invocation from `codedungeon qa`.
- Support deterministic invocation from full workflow execution.
- Persist QA sessions, checks, dependencies, artifacts, and findings.
- Keep compatibility with existing `verification_records`.
- Treat Playwright as an optional external dependency with a clear preflight model.
- Generate evidence artifacts that humans, reports, and later modules can consume.
- Return reliable statuses: `PASS`, `FAIL`, `BLOCKED`, and `SKIPPED`.
- Convert actionable failures into structured fix tasks for the execution module.

## Non-Goals

- Do not embed Playwright, browsers, Node, or browser dependencies into the Go binary.
- Do not make an LLM the authority for pass/fail.
- Do not replace every project test runner with codedungeon-specific tests.
- Do not require a full codedungeon run session for standalone QA.
- Do not silently install dependencies unless an explicit dependency-install policy is added later.

## Entrypoints

### Standalone QA

The user can run QA directly:

```powershell
codedungeon qa run --root . --auto
codedungeon qa run --root . --cmd "go test ./..."
codedungeon qa run --root . --plan .codedungeon/qa-plan.json
codedungeon qa preflight --root . --mode e2e
codedungeon qa report --session <qa-session-id>
```

Standalone mode behavior:

- If no active codedungeon run exists, QA creates a standalone QA session.
- Standalone sessions use generated IDs such as `qa-YYYYMMDD-HHMMSS-<shortid>`.
- `qa_sessions.run_id` is nullable or stores this standalone ID with `entrypoint=standalone`.
- The module writes artifacts under `.codedungeon/qa/sessions/<qa-session-id>/`.
- The command exits non-zero for `FAIL` and `BLOCKED`, unless an explicit `--allow-blocked` or report-only mode is requested.
- No agent is required. `--auto` uses deterministic framework detection and command selection.

### Workflow QA

The full workflow calls QA automatically:

```powershell
codedungeon run ...
```

Workflow mode behavior:

- The run engine creates a `qa.Request` after implementation/execution and before final report/merge gates.
- QA receives the active run/session IDs, project context, task plan, changed files, and execution outputs.
- QA runs required checks deterministically. It does not wait for an agent to decide which QA command to call.
- If QA returns `PASS`, the workflow can proceed to review/report gates.
- If QA returns `FAIL`, the workflow stops or loops into execution with structured fix tasks, depending on the configured full-cycle policy.
- If QA returns `BLOCKED`, the workflow stops with dependency/environment instructions unless the run policy permits blocked verification.
- Existing phase-6 verification semantics remain compatible through `verification_records`.

## Core Package

Package path:

```text
src/codedungeon/internal/qa
```

Primary types:

```go
type Request struct {
    Entrypoint     Entrypoint
    Root           string
    RunID          string
    ExecutionID    string
    Phase          string
    Mode           Mode
    PlanPath       string
    ProjectContext string
    TaskContext    string
    ChangedFiles   []string
    Commands       []CommandSpec
    DependencyMode DependencyMode
    OutputDir      string
    Fresh          bool
}

type Result struct {
    SessionID    string
    Status       Status
    StartedAt    time.Time
    FinishedAt   time.Time
    EvidenceDir  string
    Checks       []CheckResult
    Dependencies []DependencyResult
    Findings     []Finding
    FixTasks     []FixTask
    SummaryPath  string
    ResultPath   string
}
```

Important enums:

- `Entrypoint`: `standalone`, `workflow`.
- `Mode`: `auto`, `verify`, `unit`, `integration`, `api`, `e2e`, `full`.
- `Status`: `PASS`, `FAIL`, `BLOCKED`, `SKIPPED`.
- `DependencyMode`: `strict`, `best-effort`, `report-only`.

## Engine Flow

The engine runs the same pipeline in both entrypoints:

1. Normalize request.
2. Create QA session and evidence directory.
3. Run preflight.
4. Build or load QA plan.
5. Execute checks.
6. Parse reports and collect artifacts.
7. Analyze failures.
8. Emit findings and fix tasks.
9. Write summary/result files.
10. Persist DB records and compatibility `verification_records`.

The entrypoint changes context and gating, not the core behavior.

## QA Plan

QA plan is a machine-readable contract. It can be loaded from a file, generated deterministically from framework detection, or suggested by an agent then validated by deterministic code.

Example shape:

```json
{
  "version": 1,
  "mode": "full",
  "required_checks": [
    {
      "id": "go-test",
      "kind": "command",
      "name": "Go tests",
      "cmd": "go test ./...",
      "cwd": ".",
      "timeout_seconds": 600
    },
    {
      "id": "playwright",
      "kind": "playwright",
      "name": "Browser E2E",
      "required": true,
      "timeout_seconds": 900
    }
  ],
  "optional_checks": [],
  "artifact_policy": {
    "capture_logs": true,
    "capture_playwright_report": true,
    "capture_traces": true
  }
}
```

Plan validation rules:

- Required checks must have stable IDs.
- Commands must have cwd under the project root.
- Timeouts are required or defaulted.
- E2E checks must declare dependency requirements.
- A plan with no executable required checks cannot produce `PASS`.

## Preflight

Preflight detects:

- Repository root and writable `.codedungeon` directory.
- Git availability and dirty-state metadata.
- Language/framework indicators.
- Tool availability: `go`, `node`, `npm`, `npx`, `python`, `pytest`, `cargo`, etc.
- Optional tools: `gh`, Playwright, Docker/Podman if configured later.
- Environment requirements such as auth files, API base URLs, test env vars.

Preflight output:

```json
{
  "status": "PASS",
  "dependencies": [
    {
      "name": "playwright",
      "required": true,
      "status": "missing",
      "install_hint": "npm i -D @playwright/test && npx playwright install --with-deps"
    }
  ]
}
```

If a required dependency is missing, the session status is `BLOCKED`, not `FAIL`.

## External Dependency Model

QA dependencies follow the same operational style as `gh`: detect availability, record version/status, provide install guidance, and fail closed only when the missing tool is required for the selected mode.

Initial dependency classes:

- `required`: the selected plan cannot run without it.
- `optional`: useful if present, but not required for the selected plan.
- `workflow-required`: required only when called from full-cycle workflow policy.
- `blocked`: unavailable or misconfigured.

Examples:

- `gh` can be optional for standalone QA but workflow-required for PR/report flows.
- `playwright` is optional for unit/API verification but required for E2E checks.
- `node` and `npx` become required when Playwright is required.

## Runners

Initial runners:

- `CommandRunner`: executes shell commands with timeout, cwd, env, logs, exit code, and duration.
- `APIValidator`: ports current `qa validate-api` behavior into `internal/qa`.
- `SecretScanner`: ports current `qa secret-scan` behavior into `internal/qa`.
- `FrameworkDetector`: ports current `qa detect-framework` behavior into `internal/qa`.
- `PlaywrightRunner`: executes Playwright when present and required.

Runner contract:

```go
type Runner interface {
    Kind() string
    Preflight(ctx context.Context, req Request, check CheckSpec) DependencyResult
    Run(ctx context.Context, sess Session, check CheckSpec) CheckResult
}
```

## Playwright Integration

Playwright is supported by adapter, not embedded.

Detection:

- `package.json` contains `@playwright/test` or `playwright`.
- `playwright.config.ts`, `playwright.config.js`, or equivalent config exists.
- `npx playwright --version` succeeds.
- Browser installation is checked by attempting a dry metadata command or by interpreting the first run failure.

Execution:

- Prefer project scripts if present, for example `npm run test:e2e`.
- Otherwise run `npx playwright test`.
- Set reporter output to JSON where possible:
  - `PLAYWRIGHT_JSON_OUTPUT_FILE=<evidence-dir>/playwright/results.json`
  - `npx playwright test --reporter=json`
- Optionally run with HTML reporter output under the evidence dir.
- Use trace policy from project config. If QA needs stronger evidence, use `--trace retain-on-failure` or a documented equivalent policy.

Artifacts to collect:

- JSON result file.
- `playwright-report/`.
- `test-results/`.
- `trace.zip` files.
- Screenshots and videos.
- Console/network artifacts if generated by project tests.

Dependency handling:

- If no E2E check is required, missing Playwright is recorded as optional.
- If an E2E check is required and Playwright is missing, status is `BLOCKED`.
- Install hints are reported but not executed automatically.

Agent-facing Playwright support:

- If future agent-assisted E2E generation is enabled, use Playwright's own planner/generator/healer concepts as inspiration.
- Generated tests must still be executed by the deterministic QA engine.
- A healer may propose a test fix, but cannot mark the QA session as passed.

## Failure Analysis

Failure analysis is two-stage:

1. Deterministic classification.
2. Optional agent-assisted explanation and fix-task proposal.

Failure categories:

- `dependency_missing`
- `environment_blocked`
- `auth_missing`
- `command_failed`
- `assertion_failed`
- `timeout`
- `flake_candidate`
- `secret_found`
- `report_parse_failed`
- `artifact_missing`

Agent-assisted analysis may summarize logs and propose next actions, but it cannot override raw check status.

## Fix Tasks

When a failure is actionable, QA writes fix tasks under:

```text
.codedungeon/qa/sessions/<qa-session-id>/fix-tasks/
```

Fix task shape:

```json
{
  "version": 1,
  "source": "qa",
  "qa_session_id": "...",
  "check_id": "go-test",
  "category": "assertion_failed",
  "title": "Fix failing Go test in internal/taskexec",
  "evidence": ["logs/go-test.log", "checks/go-test.json"],
  "suggested_entrypoint": "codedungeon execute run --task <id>"
}
```

In standalone mode, fix tasks are artifacts for the user.

In workflow mode, the full-cycle runner can feed fix tasks into execution if loop policy allows it.

## Persistence

Add tables:

```sql
qa_sessions(
  id text primary key,
  run_id text,
  execution_id text,
  entrypoint text not null,
  mode text not null,
  status text not null,
  root text not null,
  plan_path text,
  evidence_dir text not null,
  started_at text not null,
  updated_at text not null,
  finished_at text,
  failure_message text
);

qa_checks(
  id text primary key,
  session_id text not null,
  kind text not null,
  name text not null,
  status text not null,
  command text,
  cwd text,
  exit_code integer,
  duration_ms integer,
  log_path text,
  report_path text,
  artifacts_json text,
  started_at text not null,
  finished_at text
);

qa_dependencies(
  id text primary key,
  session_id text not null,
  name text not null,
  required integer not null,
  status text not null,
  version text,
  install_hint text,
  detail text
);

qa_findings(
  id text primary key,
  session_id text not null,
  severity text not null,
  category text not null,
  title text not null,
  detail text,
  evidence_path text,
  fix_task_path text,
  created_at text not null
);

qa_artifacts(
  id text primary key,
  session_id text not null,
  check_id text,
  kind text not null,
  path text not null,
  sha256 text,
  bytes integer,
  created_at text not null
);
```

Keep writing `verification_records` for backward compatibility during migration.

## Evidence Layout

Default layout:

```text
.codedungeon/qa/sessions/<qa-session-id>/
  request.json
  result.json
  preflight.json
  plan.json
  summary.md
  checks/
    <check-id>.json
  logs/
    <check-id>.log
  api/
    <check-id>.json
  playwright/
    results.json
    playwright-report/
    test-results/
  findings.json
  fix-tasks/
    <fix-task-id>.json
```

Every path persisted in DB is relative to the repository root when possible.

## CLI Design

Keep existing commands and route them through `internal/qa`:

```text
codedungeon qa run
codedungeon qa record
codedungeon qa secret-scan
codedungeon qa validate-api
codedungeon qa detect-framework
```

Add or formalize:

```text
codedungeon qa preflight
codedungeon qa status
codedungeon qa report
```

`qa run` supports:

- `--root`
- `--auto`
- `--mode auto|verify|unit|integration|api|e2e|full`
- `--plan`
- `--cmd`
- `--fresh`
- `--timeout`
- `--dependency-mode strict|best-effort|report-only`
- `--json`

Standalone examples:

```powershell
codedungeon qa run --auto
codedungeon qa run --mode e2e
codedungeon qa run --cmd "go test ./..."
codedungeon qa report --latest
```

Workflow usage is internal:

```go
result, err := qaEngine.Run(ctx, qa.Request{
    Entrypoint: qa.EntrypointWorkflow,
    RunID: run.ID,
    ExecutionID: execution.ID,
    Root: root,
    Mode: qa.ModeVerify,
    Fresh: true,
})
```

## Full-Cycle Integration

Full-cycle runner responsibilities:

- Always call QA when entering verification phase.
- Pass task/execution context into QA.
- Enforce QA result according to policy.
- Never require an agent to invoke `codedungeon qa`.

Default gating:

- `PASS`: continue.
- `FAIL`: stop and surface findings; if loop policy is enabled, create execution fix tasks and retry up to configured max cycles.
- `BLOCKED`: stop with dependency/environment instructions.
- `SKIPPED`: continue only if the skipped check was optional.

This makes QA autonomous inside the workflow while preserving standalone CLI behavior.

## Error Handling

- Command non-zero exit becomes `FAIL`.
- Missing required dependency becomes `BLOCKED`.
- Timeout becomes `FAIL` unless dependency/environment evidence indicates `BLOCKED`.
- Missing required artifact after a supposed pass becomes `FAIL`.
- Report parse failure becomes `FAIL` for required structured runners.
- Panic/internal error becomes `BLOCKED` if no check ran, otherwise `FAIL` with internal error finding.

## Backward Compatibility

- Existing `codedungeon qa run --phase 6 --fresh --cmd ...` remains valid.
- Existing reports reading `verification_records` keep working.
- New QA sessions additionally write richer records.
- `qa detect-framework` keeps its current CLI behavior while delegating to `internal/qa`.
- `qa validate-api` keeps its current CLI behavior while delegating to `internal/qa`.

## Implementation Slices

1. Create `internal/qa` request/result/session model and artifact writer.
2. Move framework detection and command runner logic behind package APIs.
3. Add DB tables and repository methods.
4. Convert `cmd/qa.go` to a thin CLI wrapper.
5. Add standalone `qa run --auto`, `preflight`, `status`, and `report`.
6. Add workflow integration so full-cycle execution calls QA automatically.
7. Add Playwright adapter and artifact collection.
8. Add failure analysis and fix-task emission.
9. Add compatibility writes to `verification_records`.
10. Add tests for standalone and workflow modes.

## Testing Strategy

Unit tests:

- Request normalization.
- Plan validation.
- Dependency classification.
- Status aggregation.
- Evidence path generation.
- Failure categorization.

Integration tests:

- Standalone `qa run --cmd`.
- Standalone `qa run --auto` for Go fixture.
- Workflow invocation with a fake execution session.
- Required dependency missing produces `BLOCKED`.
- Failed command produces `FAIL`.
- Successful command produces `PASS`.
- Existing `verification_records` compatibility is preserved.

Playwright tests:

- Use a fixture project with `@playwright/test` installed only in environments where dependencies are available.
- If Playwright is not installed, assert `BLOCKED` and install hint behavior.
- If installed, assert JSON report and artifact collection.

## Acceptance Criteria

- A user can run QA alone and get a QA session, result file, summary, and exit code.
- Full-cycle run invokes QA automatically and gates on its result.
- QA can pass without an agent if deterministic checks pass.
- QA can fail without an agent if checks fail.
- Missing Playwright for required E2E is reported as `BLOCKED`, not confused with code failure.
- Existing `qa run`, `qa validate-api`, `qa secret-scan`, and `qa detect-framework` remain usable.
- Evidence is durable, inspectable, and linked from DB records.
- Reports can consume QA evidence without parsing raw terminal logs.
