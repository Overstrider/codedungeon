package prompts

import (
	"strings"
	"testing"
)

func TestClaudeArtifactsSplitNativeAndRuntimeInstallPaths(t *testing.T) {
	arts, err := ArtifactsFor("claude")
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) == 0 {
		t.Fatal("expected claude artifacts")
	}
	for _, a := range arts {
		if a.Provider != "claude" {
			t.Fatalf("provider = %q, want claude", a.Provider)
		}
		if a.PackID != "codedungeon-claude" {
			t.Fatalf("pack id = %q, want codedungeon-claude", a.PackID)
		}
		if strings.HasPrefix(a.RelPath, "phases/") || strings.HasPrefix(a.RelPath, "commands/") {
			if !strings.HasPrefix(a.InstallPath, ".codedungeon/") {
				t.Fatalf("editable install path %q should live under .codedungeon/", a.InstallPath)
			}
			continue
		}
		if strings.HasPrefix(a.RelPath, "command-wrappers/") {
			if !strings.HasPrefix(a.InstallPath, ".claude/commands/") || a.Kind != "command-wrapper" {
				t.Fatalf("wrapper install path/kind = %q/%q, want .claude/commands + command-wrapper", a.InstallPath, a.Kind)
			}
			continue
		}
		if !strings.HasPrefix(a.InstallPath, ".claude/") {
			t.Fatalf("native install path %q should stay under .claude/", a.InstallPath)
		}
	}
}

func TestClaudeLegacyPromptAliasesUseCanonicalPack(t *testing.T) {
	for _, alias := range []string{"claude-code", "claude-ce"} {
		arts, err := ArtifactsFor(alias)
		if err != nil {
			t.Fatal(err)
		}
		if len(arts) == 0 {
			t.Fatalf("expected artifacts for alias %s", alias)
		}
		if arts[0].Provider != "claude" {
			t.Fatalf("alias %s provider = %q, want claude", alias, arts[0].Provider)
		}
	}
}

func TestClaudeSpawnArtifactsRequireDangerouslySkipPermissions(t *testing.T) {
	arts, err := ArtifactsFor("claude")
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range arts {
		if !isClaudeSpawnSurface(a.RelPath) {
			continue
		}
		body := string(a.Content)
		if !mentionsClaudeSpawn(body) {
			continue
		}
		if !strings.Contains(body, "--dangerously-skip-permissions") {
			t.Fatalf("%s mentions Claude spawn flow but does not require --dangerously-skip-permissions", a.RelPath)
		}
	}
}

func isClaudeSpawnSurface(rel string) bool {
	return strings.HasPrefix(rel, "commands/") ||
		strings.HasPrefix(rel, "phases/") ||
		rel == "skills/summoning-circle-spawn/SKILL.md"
}

func mentionsClaudeSpawn(body string) bool {
	lower := strings.ToLower(body)
	return strings.Contains(lower, "spawn") ||
		strings.Contains(body, "Task tool") ||
		strings.Contains(body, "Task calls") ||
		strings.Contains(body, "general-purpose")
}

func TestCodexArtifactsAreProviderNative(t *testing.T) {
	arts, err := ArtifactsFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{
		"AGENTS.md":                                     false,
		".codex/config.toml":                            false,
		".codex/agents/cd_dev_worker.toml":              false,
		".codedungeon/commands/main-quest.md":           false,
		".codedungeon/commands/side-quest.md":           false,
		".codedungeon/commands/one-shot.md":             false,
		".codedungeon/commands/codedungeon.md":          false,
		".codedungeon/phases/forge-execution.md":        false,
		".agents/skills/codedungeon-flow/SKILL.md":      false,
		".agents/skills/codedungeon/SKILL.md":           false,
		".agents/skills/backend-specialist/SKILL.md":    false,
		".agents/skills/main-quest/SKILL.md":            false,
		".agents/skills/side-quest/SKILL.md":            false,
		".agents/skills/one-shot/SKILL.md":              false,
		".agents/skills/code-review/SKILL.md":           false,
		".agents/skills/codedungeon-test-loop/SKILL.md": false,
		".agents/skills/cleanup-tasks/SKILL.md":         false,
	}
	for _, a := range arts {
		if a.Provider != "codex" {
			t.Fatalf("provider = %q, want codex", a.Provider)
		}
		if strings.HasPrefix(a.InstallPath, ".codex/skills/") {
			t.Fatalf("codex skills must install under .agents/skills, got %q", a.InstallPath)
		}
		if strings.HasPrefix(a.RelPath, "phases/") && !strings.HasPrefix(a.InstallPath, ".codedungeon/phases/") {
			t.Fatalf("codex phases must install under .codedungeon/phases, got %q", a.InstallPath)
		}
		if strings.HasPrefix(a.RelPath, "commands/") && !strings.HasPrefix(a.InstallPath, ".codedungeon/commands/") {
			t.Fatalf("codex commands must install under .codedungeon/commands, got %q", a.InstallPath)
		}
		if strings.HasPrefix(a.InstallPath, ".codex/commands/") {
			t.Fatalf("codex commands must not install under .codex/commands, got %q", a.InstallPath)
		}
		if _, ok := want[a.InstallPath]; ok {
			want[a.InstallPath] = true
		}
	}
	for path, seen := range want {
		if !seen {
			t.Fatalf("missing codex artifact install path %q", path)
		}
	}
}

func TestCodexAgentsUseOfficialTomlSchema(t *testing.T) {
	arts, err := ArtifactsFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, a := range arts {
		if a.Kind != "agent" {
			continue
		}
		count++
		body := string(a.Content)
		if strings.HasPrefix(body, "\ufeff") {
			t.Fatalf("%s starts with UTF-8 BOM; Codex agent TOML should be plain UTF-8", a.RelPath)
		}
		for _, required := range []string{"name = ", "description = ", "developer_instructions = "} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing %q", a.RelPath, required)
			}
		}
		if strings.Contains(body, "\nprompt = ") {
			t.Fatalf("%s uses prompt field; Codex custom agents require developer_instructions", a.RelPath)
		}
		for _, required := range []string{"Role:", "Working mode:", "Return:"} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing compact instruction section %q", a.RelPath, required)
			}
		}
		for _, forbidden := range []string{
			"max_thinking_tokens",
			"--dangerously-skip-permissions",
			"model = ",
			"model_reasoning_effort",
			"sandbox_mode",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s contains provider/runtime setting %q that should stay outside Codex agent TOML", a.RelPath, forbidden)
			}
		}
	}
	if count == 0 {
		t.Fatal("expected codex agents")
	}
}

func TestCodexConfigEnablesCustomAgentSpawning(t *testing.T) {
	raw, err := GetRawFor("codex", "config.toml")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	for _, required := range []string{
		"[features]",
		"multi_agent_v2 = true",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("codex config missing %q:\n%s", required, body)
		}
	}
	for _, forbidden := range []string{
		"max_threads",
		"max_depth",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("codex config contains %q, which current Codex rejects with multi_agent_v2 enabled:\n%s", forbidden, body)
		}
	}
}

func TestCodexCommandSkillsArePrimaryWorkflowSurface(t *testing.T) {
	for _, name := range []string{
		"codedungeon",
		"main-quest",
		"side-quest",
		"one-shot",
		"code-review",
		"codedungeon-test-loop",
		"cleanup-tasks",
	} {
		raw, err := GetRawFor("codex", "skills/"+name+"/SKILL.md")
		if err != nil {
			t.Fatalf("missing command skill %s: %v", name, err)
		}
		body := string(raw)
		if !strings.Contains(body, "name: "+name) {
			t.Fatalf("skill %s missing matching frontmatter name", name)
		}
		if strings.Contains(body, "slash command") {
			t.Fatalf("skill %s should not describe itself as a slash command", name)
		}
	}
}

func TestCodexSkillsStartWithValidFrontmatter(t *testing.T) {
	arts, err := ArtifactsFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	for _, a := range arts {
		if a.Kind != "skill" || !strings.HasSuffix(a.RelPath, "/SKILL.md") {
			continue
		}
		count++
		body := string(a.Content)
		if !strings.HasPrefix(body, "---\n") && !strings.HasPrefix(body, "---\r\n") {
			t.Fatalf("%s must start with YAML frontmatter", a.RelPath)
		}
		normalized := strings.ReplaceAll(body, "\r\n", "\n")
		rest := strings.TrimPrefix(normalized, "---\n")
		end := strings.Index(rest, "\n---\n")
		if end < 0 {
			t.Fatalf("%s missing closing YAML frontmatter fence", a.RelPath)
		}
		frontmatter := rest[:end]
		if !strings.Contains(frontmatter, "\nname: ") && !strings.HasPrefix(frontmatter, "name: ") {
			t.Fatalf("%s frontmatter missing name field:\n%s", a.RelPath, frontmatter)
		}
		if !strings.Contains(frontmatter, "\ndescription: ") && !strings.HasPrefix(frontmatter, "description: ") {
			t.Fatalf("%s frontmatter missing description field:\n%s", a.RelPath, frontmatter)
		}
		if strings.Contains(frontmatter, "## ") || strings.Contains(frontmatter, "```") {
			t.Fatalf("%s frontmatter contains markdown body content:\n%s", a.RelPath, frontmatter)
		}
		for _, line := range strings.Split(frontmatter, "\n") {
			value, ok := strings.CutPrefix(line, "description: ")
			if ok && !strings.HasPrefix(value, `"`) && strings.Contains(value, ": ") {
				t.Fatalf("%s frontmatter description with colon-space must be quoted:\n%s", a.RelPath, line)
			}
		}
	}
	if count == 0 {
		t.Fatal("expected codex skill artifacts")
	}
}

func TestUnifiedRouterPromptsDeclareModesAndAliases(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
		surface  string
	}{
		{"claude", "commands/codedungeon.md", "/codedungeon"},
		{"codex", "commands/codedungeon.md", "$codedungeon"},
		{"codex", "skills/codedungeon/SKILL.md", "$codedungeon"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"--full",
			"--lite",
			"--oneshot",
			"--one-shot",
			"--auto",
			"--rules",
			"CODEDUNGEON_MODE_SELECTED:",
			"multiple mode flags",
			"Project Rules Discovery",
			"may run without a user prompt",
			"main-quest",
			"side-quest",
			"one-shot",
			tc.surface,
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing router contract %q:\n%s", tc.provider, tc.rel, required, body)
			}
		}
	}
}

func TestWorkflowsReadProjectRulesCompactAndEmitEnvelope(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
	}{
		{"claude", "commands/main-quest.md"},
		{"claude", "commands/side-quest.md"},
		{"claude", "commands/one-shot.md"},
		{"claude", "commands/codedungeon-loop.md"},
		{"claude", "commands/code-review.md"},
		{"codex", "commands/main-quest.md"},
		{"codex", "commands/side-quest.md"},
		{"codex", "commands/one-shot.md"},
		{"codex", "commands/codedungeon-loop.md"},
		{"codex", "commands/code-review.md"},
		{"codex", "skills/codedungeon/SKILL.md"},
		{"codex", "skills/main-quest/SKILL.md"},
		{"codex", "skills/side-quest/SKILL.md"},
		{"codex", "skills/one-shot/SKILL.md"},
		{"codex", "skills/code-review/SKILL.md"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			".codedungeon/project-rules.compact.md",
			"PROJECT_RULES_STATUS",
			"PROJECT_RULES_DIGEST",
			"PROJECT_RULES_READ",
			"codedungeon rules status",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing project rules contract %q:\n%s", tc.provider, tc.rel, required, body)
			}
		}
	}
}

func TestCodexPromptListIsNamespaced(t *testing.T) {
	names, err := ListFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatal("expected codex prompt names")
	}
	for _, name := range names {
		if !strings.HasPrefix(name, "codex:") {
			t.Fatalf("codex prompt name %q should be namespaced", name)
		}
	}
	body, err := GetFor("codex", "codex:caveman-ultra")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "compact") {
		t.Fatalf("unexpected codex caveman prompt body: %q", body)
	}
}

func TestCodexPromptListExcludesInstallOnlyArtifacts(t *testing.T) {
	names, err := ListFor("codex")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		switch name {
		case "codex:config", "codex:AGENTS":
			t.Fatalf("install-only artifact %q should not be seeded as a prompt", name)
		}
	}
}

func TestCodexWorkflowPromptsUseProjectLocalBinary(t *testing.T) {
	for _, rel := range []string{
		"AGENTS.md",
		"skills/codedungeon-cli/SKILL.md",
		"skills/codedungeon/SKILL.md",
		"commands/codedungeon.md",
		"skills/main-quest/SKILL.md",
		"commands/main-quest.md",
		"skills/one-shot/SKILL.md",
		"commands/one-shot.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		if !strings.Contains(body, "./.codex/bin/codedungeon") {
			t.Fatalf("%s should tell Codex to use the project-local binary", rel)
		}
	}
}

func TestOneShotCreatesBranchBeforeGuardAndReusesPR(t *testing.T) {
	raw, err := GetRawFor("claude", "commands/one-shot.md")
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)
	switchIdx := strings.Index(body, "git switch -c")
	guardIdx := strings.Index(body, "$CD git guard --repo .")
	if switchIdx == -1 {
		t.Fatal("claude one-shot should create or switch to a feature branch")
	}
	if guardIdx == -1 {
		t.Fatal("claude one-shot should run git guard after branch setup")
	}
	if guardIdx < switchIdx {
		t.Fatalf("claude one-shot runs git guard before branch setup:\n%s", body)
	}
	for _, required := range []string{
		"PR_URL=$(gh pr view --json url -q .url 2>/dev/null || true)",
		`if [ -z "$PR_URL" ]; then`,
		"PR_URL=$(gh pr create --fill)",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("claude one-shot missing PR reuse/create block %q", required)
		}
	}

	for _, rel := range []string{
		"commands/one-shot.md",
		"skills/one-shot/SKILL.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		branchIdx := strings.Index(body, "Create or")
		guardIdx := strings.Index(body, "git guard")
		if branchIdx == -1 || guardIdx == -1 {
			t.Fatalf("%s should mention branch setup and git guard:\n%s", rel, body)
		}
		if guardIdx < branchIdx {
			t.Fatalf("%s should mention git guard only after branch setup:\n%s", rel, body)
		}
		if !strings.Contains(body, "reuse") || !strings.Contains(body, "otherwise create one") {
			t.Fatalf("%s should tell Codex to reuse an existing PR before creating one:\n%s", rel, body)
		}
	}
}

func TestPRProducingWorkflowsRequireReviewedPRReport(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
	}{
		{"claude", "commands/one-shot.md"},
		{"claude", "commands/side-quest.md"},
		{"claude", "commands/codedungeon-loop.md"},
		{"claude", "phases/forge-execution.md"},
		{"claude", "phases/throne-room-report.md"},
		{"codex", "commands/one-shot.md"},
		{"codex", "commands/side-quest.md"},
		{"codex", "skills/one-shot/SKILL.md"},
		{"codex", "skills/side-quest/SKILL.md"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"CodeDungeon PR Report",
			"Status        COMPLETE|BLOCKED|MAX_CYCLES_REACHED",
			"PR            #",
			"Review        APPROVED|CHANGES_REQUESTED|MAX_CYCLES_REACHED|NOT_RUN",
			"Cycles        ",
			"Summary",
			"Work Done",
			"Next",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing required PR report field %q", tc.provider, tc.rel, required)
			}
		}
		if !strings.Contains(body, "COMPLETE") || !strings.Contains(body, "APPROVED") {
			t.Fatalf("%s:%s should tie completion to approved review", tc.provider, tc.rel)
		}
	}
}

func TestReviewCyclesUseReducedModeAfterThirdCycle(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
	}{
		{"claude", "commands/one-shot.md"},
		{"claude", "commands/codedungeon-loop.md"},
		{"claude", "commands/code-review.md"},
		{"claude", "phases/forge-execution.md"},
		{"codex", "commands/one-shot.md"},
		{"codex", "commands/side-quest.md"},
		{"codex", "commands/code-review.md"},
		{"codex", "skills/one-shot/SKILL.md"},
		{"codex", "skills/side-quest/SKILL.md"},
		{"codex", "skills/code-review/SKILL.md"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{"1-3", "4-9", "reduced", "fast"} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing review cycle contract %q", tc.provider, tc.rel, required)
			}
		}
	}
}

func TestImplementationWorkflowsRequireVerificationGateBeforeComplete(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
	}{
		{"claude", "commands/codedungeon-loop.md"},
		{"codex", "commands/codedungeon-loop.md"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"Verification Gate",
			"codedungeon qa detect-framework",
			"cargo check",
			"cargo test",
			"Dockerfile",
			"Containerfile",
			"podman build",
			"APPROVED does not replace verification",
			"Status COMPLETE",
			"Verification: PASS",
			"Status BLOCKED",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing verification gate contract %q", tc.provider, tc.rel, required)
			}
		}
	}
}

func TestCodeReviewPromptsTreatMissingVerificationAsBlocking(t *testing.T) {
	for _, tc := range []struct {
		provider string
		rel      string
	}{
		{"claude", "commands/code-review.md"},
		{"codex", "commands/code-review.md"},
		{"codex", "skills/code-review/SKILL.md"},
	} {
		raw, err := GetRawFor(tc.provider, tc.rel)
		if err != nil {
			t.Fatalf("read %s:%s: %v", tc.provider, tc.rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"missing verification",
			"BLOCKING",
			"build/check/test",
			"APPROVED does not replace verification",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s:%s missing missing-verification review contract %q", tc.provider, tc.rel, required)
			}
		}
	}
}

func TestCodexWorkflowPromptsDeclareDeterministicCompletionGates(t *testing.T) {
	for _, rel := range []string{
		"commands/main-quest.md",
		"commands/side-quest.md",
		"commands/one-shot.md",
		"skills/main-quest/SKILL.md",
		"skills/side-quest/SKILL.md",
		"skills/one-shot/SKILL.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"Do not write review reports manually",
			"Do not write final reports manually",
			"codedungeon review run",
			"review-manifest.json",
			"codedungeon qa record",
			"codedungeon report render",
			"COMPLETE can only come from `codedungeon report render`",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing deterministic gate instruction %q:\n%s", rel, required, body)
			}
		}
	}

	for _, rel := range []string{
		"commands/code-review.md",
		"skills/code-review/SKILL.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		for _, required := range []string{
			"Do not write review reports manually",
			"review-manifest.json",
			"findings-saboteur.json",
			"codedungeon review run",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing deterministic review instruction %q:\n%s", rel, required, body)
			}
		}
	}
}

func TestReportTemplatesRenderPRReportFields(t *testing.T) {
	for _, provider := range []string{"claude", "codex"} {
		for _, name := range []string{"report-template-multi", "report-template-bootstrap"} {
			body, err := GetFor(provider, name)
			if err != nil {
				t.Fatalf("read %s:%s: %v", provider, name, err)
			}
			for _, required := range []string{
				"CodeDungeon PR Report",
				"Status",
				"Workflow",
				"PR",
				"Review",
				"Cycles",
				"Work Done",
				"Next",
			} {
				if !strings.Contains(body, required) {
					t.Fatalf("%s:%s missing %q", provider, name, required)
				}
			}
		}
	}
}

func TestCodexFullWorkflowDocumentsFeatureInitAndGitHubPrereqs(t *testing.T) {
	for _, rel := range []string{
		"commands/main-quest.md",
		"skills/main-quest/SKILL.md",
		"commands/codedungeon.md",
		"skills/codedungeon/SKILL.md",
	} {
		raw, err := GetRawFor("codex", rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		body := string(raw)
		if strings.Contains(body, "phase init` if") || strings.Contains(body, "phase init if") {
			t.Fatalf("%s documents phase init without --feature:\n%s", rel, body)
		}
		for _, required := range []string{
			"phase init --feature",
			"gh auth status",
			"git remote get-url origin",
			"GitHub PR",
		} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s missing GitHub/phase-init instruction %q:\n%s", rel, required, body)
			}
		}
	}
}
