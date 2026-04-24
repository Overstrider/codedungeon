# Code Review (Adversarial — Opus 4.7 Fanout)

Runs an **adversarial PR code review** on the current branch using Claude Opus 4.7 in a multi-persona fanout (Saboteur + New Hire + Security Auditor + Spec Enforcer) followed by per-finding Sonnet validators and a stack-specific `{LANG}-specialist` pass. Posts results to GitHub as a PR comment with severity tiers P0/P1/P2 and a machine-parseable tally.

**Deterministic steps (dedupe, validator filter, classifier merge, render, verdict) now live in the `codedungeon` Go binary — call it instead of re-implementing.** Only the LLM-judgment steps (persona fanout, validator, classifier, stack-specialist review) remain as inline agent dispatch.

## Parameters

- `$ARGUMENTS` — Repository path (absolute) OR repo name.

## Why adversarial fanout?

Research (Anthropic code-review plugin, Greptile benchmarks 82%, Meta MetaMateCR arXiv 2507.13499, SpecterOps, arXiv 2509.16533 on sycophancy) shows single-agent self-review misses real bugs. Fix: **session separation + multi-persona fanout + per-finding validator + confidence tiers + quote-as-evidence anti-hallucination contract**.

Personas run in parallel (recall). Validators run per-finding on Sonnet (precision, cross-model reduces sycophancy on Opus findings). Severity promotes when ≥2 personas flag the same issue (handled by `codedungeon review run --only dedupe`). Design-decision classifier inspects each validated finding against CLAUDE.md/REVIEW.md/ADRs to separate `actionable` from `design_decision` — APPROVED requires every remaining finding to be a documented design decision.

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

## YOUR ROLE
Read your full instructions from your agent definition ({persona-name}).
Output ONLY valid JSON matching the schema in your agent definition.
Write it to {OUTPUT_PATH}.
```

Dispatch:
- `subagent_type: review-saboteur`        → `.claude/plan/adv-review/findings-saboteur.json`
- `subagent_type: review-newhire`         → `.claude/plan/adv-review/findings-newhire.json`
- `subagent_type: review-security-auditor` → `.claude/plan/adv-review/findings-security.json`
- `subagent_type: review-spec-enforcer`   → `.claude/plan/adv-review/findings-spec.json`

## Step 6: Dedupe + severity promotion (CLI)

```bash
codedungeon review run --only dedupe --dir "$REPO_DIR/.claude/plan/adv-review"
```

Writes `findings-merged.json` with `flagged_by: [...]` per finding. ≥2 personas on same (file, category, overlapping lines) → severity promoted one tier (capped P0). P2 extras over `--nit-cap 3` roll into `suppressed_nits_count`.

## Step 7: Per-finding validator (LLM — parallel, Sonnet)

For each merged finding, spawn a `review-validator` subagent (Sonnet). Batch up to 10 parallel Task calls per message. Validator writes `.claude/plan/adv-review/validator-<idx>.json` per input.

Then:

```bash
codedungeon review run --only filter --dir "$REPO_DIR/.claude/plan/adv-review"
```

Drops `confirmed:false` + `confidence:low`.

## Step 7.5: Design-decision classifier (LLM — parallel, Sonnet)

Resolve classifier context paths:

```bash
codedungeon review context-paths --repo "$REPO_DIR" > /tmp/ctx.json
# → claude_md_root, claude_md_repo, review_md, architecture_md, adr_paths, spec_md, task_files
```

Spawn `review-design-classifier` per finding (batches of 10). Each reads the context paths + one finding JSON. Writes `classifier-<idx>.json`.

Then:

```bash
codedungeon review run --only classify --dir "$REPO_DIR/.claude/plan/adv-review"
```

Merge rule: `classification=design_decision && confidence=high` → `actionable=false`; else `actionable=true`. **Hard override**: `severity=P0 && confidence≠high` → force `actionable=true`.

## Step 8: Stack-specialist pass (LLM)

Spawn `{LANG}-specialist` in CODE REVIEW mode with `findings-classified.json` as prior context. Agent adds NEW findings (stack-specific rubric) only; does NOT duplicate. Writes `findings-stack.json`.

Classify the stack findings (same classifier flow as Step 7.5) → `classifier-stack-<idx>.json`.

Then the final merge + render:

```bash
codedungeon review run --dir "$REPO_DIR/.claude/plan/adv-review" \
  --validator-model "sonnet-4.6" --classifier-model "sonnet-4.6" \
  --stack-specialist "${LANG}-specialist" > /tmp/verdict.json
```

This runs classify (including stack findings) + render + verdict in one shot. Output:

```json
{"ok":true,"verdict":"APPROVED|CHANGES_REQUESTED","tally":{...},"review_md":".../review.md","review_json":".../review.json","personas":["saboteur","newhire","security","spec"]}
```

## Step 10: Post to GitHub PR

```bash
gh pr comment "$PR_NUM" --body "$(cat "$REPO_DIR/.claude/plan/adv-review/review.md")

---
*Automated adversarial review — Claude Opus 4.7 (personas) + Sonnet 4.6 (validators)*"
```

The title line `## Claude Adversarial Code Review` is LOAD-BEARING — `phase-5-execution.md` greps for it.

## Step 11: Report verdict

```bash
jq -r '.verdict' /tmp/verdict.json
jq -r '.tally' /tmp/verdict.json
```

Return verdict to caller (`codedungeon-loop`, `phase-5-execution`).

---

## Notes

- **Anti-hallucination**: three layers (persona quote requirement, Validator re-read, the title regex in phase-5 verification).
- **Cross-model split**: Opus 4.7 for personas (recall); Sonnet 4.6 for Validator and Design-Decision Classifier (precision, cost, anti-sycophancy).
- **Output paths**: all intermediates under `<REPO>/.claude/plan/adv-review/` — safe to gitignore at repo level.
- **Severity tiers**: P0 Important, P1 Should-fix, P2 Nit. **All three block** unless classified as design decision.
- **Design-decision escape hatch**: documented in REVIEW.md / CLAUDE.md / ADRs / spec / `// INTENTIONAL:` comments. `TODO`/`FIXME`/`HACK` do NOT count.
- **Extensibility**: per-repo `REVIEW.md` overrides severity calibration, nit cap, skip-rules, threat model. Template (installed by bootstrap): `.claude/commands/templates/REVIEW.md.template` (fallback: `$HOME/.claude/plugins/local/codedungeon/commands/templates/REVIEW.md.template`).
