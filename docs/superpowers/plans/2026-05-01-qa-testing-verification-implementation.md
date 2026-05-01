# QA Testing Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a first-class `internal/qa` module that runs standalone from `codedungeon qa` and gates full-cycle workflows automatically.

**Architecture:** Add a deterministic QA engine with request/result contracts, preflight, plan execution, evidence artifacts, Playwright dependency detection, SQLite persistence, and compatibility writes to `verification_records`. Keep existing helper commands compatible while moving framework detection and verification execution into `internal/qa`.

**Tech Stack:** Go 1.25, cobra CLI, SQLite via `internal/db`, existing `osadapter`, existing CodeDungeon runtime paths.

---

### Task 1: Core QA Package

**Files:**
- Create: `src/codedungeon/internal/qa/types.go`
- Create: `src/codedungeon/internal/qa/engine.go`
- Create: `src/codedungeon/internal/qa/framework.go`
- Create: `src/codedungeon/internal/qa/engine_test.go`

- [ ] **Step 1: Write failing tests**

Add tests proving:

```go
func TestEngineRunsStandaloneCommandAndWritesEvidence(t *testing.T)
func TestEngineClassifiesMissingPlaywrightAsBlockedForE2E(t *testing.T)
func TestDetectFrameworkDiscoversMonorepoCommands(t *testing.T)
```

Run: `go test ./internal/qa`
Expected: FAIL because package/types are missing.

- [ ] **Step 2: Implement minimal core**

Implement:

```go
type Request struct { Root string; RunID int64; Entrypoint Entrypoint; Mode Mode; Phase string; Commands []CommandSpec; Fresh bool; Store Store }
type Result struct { SessionID string; Status Status; EvidenceDir string; Checks []CheckResult; Dependencies []DependencyResult; Findings []Finding; SummaryPath string; ResultPath string }
func Run(ctx context.Context, req Request) (Result, error)
func DetectFramework(root string) FrameworkResult
```

Run: `go test ./internal/qa`
Expected: PASS.

### Task 2: QA Persistence

**Files:**
- Modify: `src/codedungeon/internal/db/schema.sql`
- Add: `src/codedungeon/internal/db/migrations/015_qa_sessions.sql`
- Modify: `src/codedungeon/internal/db/store.go`
- Add: `src/codedungeon/internal/db/qa_test.go`

- [ ] **Step 1: Write failing persistence test**

Add:

```go
func TestStorePersistsQASessionChecksDependenciesAndFindings(t *testing.T)
```

Run: `go test ./internal/db -run TestStorePersistsQASession`
Expected: FAIL because QA tables/helpers do not exist.

- [ ] **Step 2: Implement schema and helpers**

Add schema version 15 and helpers:

```go
func (s *Store) UpsertQASession(QASession) error
func (s *Store) InsertQACheck(QACheck) (int64, error)
func (s *Store) InsertQADependency(QADependency) (int64, error)
func (s *Store) InsertQAFinding(QAFinding) (int64, error)
func (s *Store) LatestQASession(runID int64) (*QASession, error)
func (s *Store) QASession(id string) (*QASession, error)
```

Run: `go test ./internal/db -run TestStorePersistsQASession`
Expected: PASS.

### Task 3: CLI QA Standalone

**Files:**
- Modify: `src/codedungeon/cmd/qa.go`
- Modify: `src/codedungeon/cmd/qa_test.go`

- [ ] **Step 1: Write failing CLI tests**

Add tests proving:

```go
func TestQARunStandaloneAutoDoesNotRequireActiveRun(t *testing.T)
func TestQAPreflightE2EReportsMissingPlaywrightAsBlocked(t *testing.T)
```

Run: `go test ./cmd -run "TestQARunStandalone|TestQAPreflight"`
Expected: FAIL because flags/subcommands do not exist or still require active run.

- [ ] **Step 2: Route CLI through internal QA**

Implement:

```text
codedungeon qa run --auto
codedungeon qa run --mode e2e
codedungeon qa run --cmd "<command>"
codedungeon qa preflight
codedungeon qa status --latest
codedungeon qa report --latest
```

Run: `go test ./cmd -run "TestQARun|TestQAPreflight"`
Expected: PASS.

### Task 4: Workflow QA Gate

**Files:**
- Modify: `src/codedungeon/cmd/run.go`
- Add or modify: `src/codedungeon/cmd/run_test.go`

- [ ] **Step 1: Write failing workflow test**

Add:

```go
func TestFinalizeRunInvokesWorkflowQAWhenVerificationMissing(t *testing.T)
```

Run: `go test ./cmd -run TestFinalizeRunInvokesWorkflowQA`
Expected: FAIL because finalization still depends on pre-existing verification records.

- [ ] **Step 2: Add deterministic workflow QA**

Before final report preparation, call:

```go
qa.Run(ctx, qa.Request{Entrypoint: qa.EntrypointWorkflow, Root: root, RunID: run.ID, Phase: "6", Mode: qa.ModeAuto, Fresh: true, Store: s})
```

Only continue if QA status is `PASS`.

Run: `go test ./cmd -run TestFinalizeRunInvokesWorkflowQA`
Expected: PASS.

### Task 5: Full Verification

**Files:**
- Modify docs where CLI behavior changed:
  - `README.md`
  - `docs/WORKFLOWS.md`

- [ ] **Step 1: Run focused tests**

Run:

```powershell
go test ./internal/qa ./internal/db ./cmd
```

Expected: PASS.

- [ ] **Step 2: Run full tests**

Run from `src/codedungeon`:

```powershell
go test ./...
```

Expected: PASS.

- [ ] **Step 3: Repository checks**

Run:

```powershell
git diff --check
```

Expected: no output.

- [ ] **Step 4: Release build**

Run from repo root:

```powershell
.\scripts\build-release.ps1
```

Expected: release binaries rebuilt successfully.
