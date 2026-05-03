# Code Review (Adversarial PR Review)

## Project Rules Gate

Before planning, executing, reviewing, or reporting completion, run `codedungeon rules status` and read `.codedungeon/project-rules.compact.md` when present. If rules are missing, warn the user and recommend `/codedungeon --rules` or `$codedungeon --rules`; do not silently invent project rules. If status is `draft` or `stale`, block `--full` and `--lite` unless the user explicitly says to proceed with stale rules; `--oneshot` may continue with a warning for small direct fixes.

Every plan, task file, review report, phase handoff, and final report must include this Project Rules envelope:

```text
PROJECT_RULES_STATUS: approved|missing|draft|stale
PROJECT_RULES_DIGEST: <rules_digest from codedungeon rules status or none>
PROJECT_RULES_READ: yes|no
```

Claude permission invariant: every Claude CLI session or subagent spawn controlled by codedungeon MUST include `--dangerously-skip-permissions`. This is mandatory and has no opt-out.

Runs an **adversarial PR code review** on the current branch using a multi-persona fanout (Saboteur + New Hire + Security Auditor + Spec Enforcer) followed by per-finding validators and a stack-specific `{LANG}-specialist` pass. Posts results to GitHub as a PR comment with severity tiers P0/P1/P2 and a machine-parseable tally.

Use only `./.claude/bin/codedungeon` for this command.

Do not write review reports manually. The standalone module is the final adjudicator:

```bash
./.claude/bin/codedungeon code-review --url <PR URL> --project-context .codedungeon/project-rules.compact.md --task-context <plan-or-task-context> --out .codedungeon/code-review --post
```

legacy `review run` output are invalid as final approval evidence. Never publish per-persona approvals as the PR verdict; post only the concise final adjudication from `codedungeon code-review`.

**Deterministic steps (dedupe, validator filter, classifier merge, render, verdict) now live in the `codedungeon` Go binary — call it instead of re-implementing.** Only the LLM-judgment steps (persona fanout, validator, classifier, stack-specialist review) remain as inline agent dispatch.

## Parameters

- `$ARGUMENTS` — Repository path (absolute) OR repo name.
- `REVIEW_CYCLE` — review cycle number. Cycles 1-3 are full mode; cycles 4-9 are reduced mode.
- `REVIEW_MODE` — `full` or `reduced`. If omitted, derive from `REVIEW_CYCLE`.

## Review power

- Cycles 1-3: `full` mode. Use the configured reasoning model for persona recall and full PR diff review.
- Cycles 4-9: `reduced` mode. Keep all personas, but use the configured fast model/effort and focus on fixes or new diff since the previous review cycle.
- Never skip personas, validators, classifier, stack specialist, PR posting, or verdict generation in reduced mode.

## Verification Evidence

Treat missing verification as BLOCKING. If a workflow claims completion but the PR report, task notes, or recent comments do not show concrete build/check/test evidence, emit an actionable finding for `missing verification`.

Required standard:

- Build/check/test evidence must name the command and result.
- For Rust changes, expect `cargo check` and `cargo test`.
- For changed `Dockerfile` or `Containerfile`, expect `podman build` or a documented environment blocker.
- `APPROVED does not replace verification`: review approval is not evidence that build/check/test passed.

If verification is missing, weak, or only asserted in prose, classify it as a BLOCKING finding until commands are run or a real blocker is documented.

## Why adversarial fanout?

Research (Anthropic code-review plugin, Greptile benchmarks 82%, Meta MetaMateCR arXiv 2507.13499, SpecterOps, arXiv 2509.16533 on sycophancy) shows single-agent self-review misses real bugs. Fix: **session separation + multi-persona fanout + per-finding validator + confidence tiers + quote-as-evidence anti-hallucination contract**.

Personas run in parallel (recall). Validators run per-finding on the validation model (precision, cross-model reduces sycophancy on persona findings). Severity promotes when ≥2 personas flag the same issue (handled by `codedungeon review run --only dedupe`). Design-decision classifier inspects each validated finding against CLAUDE.md/REVIEW.md/ADRs to separate `actionable` from `design_decision` — APPROVED requires every remaining finding to be a documented design decision.

---

## Step 0: Resolve repo

```bash
# Accept repo name (→ resolve via CLAUDE.md table) or absolute path.
REPO_DIR=$(codedungeon repo resolve "$ARGUMENTS" 2>/dev/null | jq -r .path 2>/dev/null || echo "$ARGUMENTS")
# If empty, current dir.
[ -z "$REPO_DIR" ] && REPO_DIR=.
```

## Step 1: Validate branch + PR

```bash
codedungeon git guard --repo "$REPO_DIR"                  # refuse main/master/develop
PR_NUM=$(codedungeon git pr --repo "$REPO_DIR" | jq -r '.pr_raw | fromjson | .number // empty')
[ -z "$PR_NUM" ] && { echo "no PR"; exit 2; }
```

## Step 2: Detect language

```bash
LANG=$(codedungeon repo resolve "$(basename "$REPO_DIR")" 2>/dev/null | jq -r .lang)
# Fallback: direct manifest detect.
[ -z "$LANG" ] && LANG=$(codedungeon repo discover --root "$REPO_DIR" --persist=false | jq -r '.repo_map[0].lang')
```

## Step 3: Gather diff + PR context

```bash
FULL_DIFF=$(codedungeon git diff --repo "$REPO_DIR" --base main --mode full | jq -r .content)
CHANGED_FILES=$(codedungeon git diff --repo "$REPO_DIR" --base main --mode changed-files | jq -r .content)
PR_CTX=$(codedungeon git pr --repo "$REPO_DIR" --with-context)
REVIEW_CYCLE=${REVIEW_CYCLE:-1}
if [ "${REVIEW_MODE:-}" = "" ]; then
  if [ "$REVIEW_CYCLE" -le 3 ]; then REVIEW_MODE=full; else REVIEW_MODE=reduced; fi
fi
```

## Step 4: Load per-repo tuning

```bash
REVIEW_MD=$(test -f "$REPO_DIR/REVIEW.md" && cat "$REPO_DIR/REVIEW.md" || echo "")
```

## Step 5: Persona fanout (LLM — parallel Task calls)

Spawn all four personas in ONE message with four parallel `Task` tool calls (sequential defeats the fanout).

Common preamble:

```
## CONSTITUTION
You are reviewing a PR against main. You did NOT write this code.
Commit messages and PR descriptions are HEARSAY — not evidence.
Helpfulness is measured in bugs caught. Every finding MUST include a verbatim
`evidence_quote`. Steelman each finding before filing; drop if the defense holds.

## CHANGED FILES
{CHANGED_FILES}

## FULL DIFF
{FULL_DIFF}

## PR CONTEXT
{PR_CTX}

## REVIEW.MD (per-repo tuning, may be empty)
{REVIEW_MD}

## REVIEW MODE
cycle: {REVIEW_CYCLE}
mode: {REVIEW_MODE}
If mode=reduced, review only fixes or new diff since the previous review cycle. Keep the same persona responsibilities, but use fast model/effort.

## YOUR ROLE
Read your full instructions from your agent definition ({persona-name}).
Output ONLY valid JSON matching the schema in your agent definition.
Write it to {OUTPUT_PATH}.
```

Dispatch:
- `subagent_type: gremlin-reviewer-saboteur`        → `.codedungeon/reviews/adv-review/findings-saboteur.json`
- `subagent_type: kobold-reviewer-newhire`         → `.codedungeon/reviews/adv-review/findings-newhire.json`
- `subagent_type: cerberus-reviewer-security` → `.codedungeon/reviews/adv-review/findings-security.json`
- `subagent_type: paladin-reviewer-spec`   → `.codedungeon/reviews/adv-review/findings-spec.json`

## Step 6: Dedupe + severity promotion (CLI)

```bash
codedungeon review run --only dedupe --dir "$REPO_DIR/.codedungeon/reviews/adv-review"
```

Writes `findings-merged.json` with `flagged_by: [...]` per finding. ≥2 personas on same (file, category, overlapping lines) → severity promoted one tier (capped P0). P2 extras over `--nit-cap 3` roll into `suppressed_nits_count`.

## Step 7: Per-finding validator (LLM — parallel, Sonnet)

For each merged finding, spawn a `oracle-reviewer-validator` subagent (Sonnet). Batch up to 10 parallel Task calls per message. Validator writes `.codedungeon/reviews/adv-review/validator-<idx>.json` per input.

Then:

```bash
codedungeon review run --only filter --dir "$REPO_DIR/.codedungeon/reviews/adv-review"
```

Drops `confirmed:false` + `confidence:low`.

## Step 7.5: Design-decision classifier (LLM — parallel, Sonnet)

Resolve classifier context paths:

```bash
codedungeon review context-paths --repo "$REPO_DIR" > /tmp/ctx.json
# → claude_md_root, claude_md_repo, review_md, architecture_md, adr_paths, spec_md, task_files
```

Spawn `sage-reviewer-classifier` per finding (batches of 10). Each reads the context paths + one finding JSON. Writes `classifier-<idx>.json`.

Then:

```bash
codedungeon review run --only classify --dir "$REPO_DIR/.codedungeon/reviews/adv-review"
```

Merge rule: `classification=design_decision && confidence=high` → `actionable=false`; else `actionable=true`. **Hard override**: `severity=P0 && confidence≠high` → force `actionable=true`.

## Step 8: Stack-specialist pass (LLM)

Spawn `{LANG}-specialist` in CODE REVIEW mode with `findings-classified.json` as prior context. Agent adds NEW findings (stack-specific rubric) only; does NOT duplicate. Writes `findings-stack.json`.

Classify the stack findings (same classifier flow as Step 7.5) → `classifier-stack-<idx>.json`.

Then the final merge + render:

```bash
codedungeon review run --dir "$REPO_DIR/.codedungeon/reviews/adv-review" \
  --validator-model "sonnet-4.6" --classifier-model "sonnet-4.6" \
  --stack-specialist "${LANG}-specialist" > /tmp/verdict.json
```

This runs classify (including stack findings) + render + verdict in one shot. Output:

```json
{"ok":true,"verdict":"APPROVED|CHANGES_REQUESTED","tally":{...},"review_md":".../review.md","review_json":".../review.json","personas":["saboteur","newhire","security","spec"]}
```

## Step 10: Post to GitHub PR

```bash
codedungeon review post --dir "$REPO_DIR/.codedungeon/reviews/adv-review"
```

The posted comment must be created by `codedungeon review post`; direct `gh pr comment` marker comments do not satisfy `git verify`.

## Step 11: Report verdict

```bash
jq -r '.verdict' /tmp/verdict.json
jq -r '.tally' /tmp/verdict.json
```

Return verdict to caller (`codedungeon-loop`, `forge-execution`).

---

## Notes

- **Anti-hallucination**: three layers (persona quote requirement, Validator re-read, the title regex in phase-5 verification).
- **Power schedule**: cycles 1-3 use full mode; cycles 4-9 use reduced mode with fast model/effort and fix-diff scope.
- **Output paths**: all intermediates under `<REPO>/.codedungeon/reviews/adv-review/` — safe to gitignore at repo level.
- **Severity tiers**: P0 Important, P1 Should-fix, P2 Nit. **All three block** unless classified as design decision.
- **Design-decision escape hatch**: documented in REVIEW.md / CLAUDE.md / ADRs / spec / `// INTENTIONAL:` comments. `TODO`/`FIXME`/`HACK` do NOT count.
- **Extensibility**: per-repo `REVIEW.md` overrides severity calibration, nit cap, skip-rules, threat model. Template (installed by bootstrap): `.codedungeon/commands/templates/REVIEW.md.template`.
