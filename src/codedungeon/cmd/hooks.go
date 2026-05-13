package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func HooksCmd() *cobra.Command {
	c := &cobra.Command{Use: "hooks", Short: "Install provider hook adapters"}
	c.AddCommand(hooksInstallCmd())
	return c
}

func hooksInstallCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "install",
		Short: "Install Project Rules hooks for Claude or Codex",
		RunE: func(c *cobra.Command, _ []string) error {
			root := ResolveProjectRoot(mustGetwd())
			providerName, _ := c.Flags().GetString("provider")
			mode, _ := c.Flags().GetString("mode")
			if err := installProjectRulesHooks(root, providerName, mode); err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{
				"ok":       true,
				"provider": providerName,
				"mode":     mode,
			})
		},
	}
	c.Flags().String("provider", "codex", "provider hook adapter: codex or claude")
	c.Flags().String("mode", "warn", "warn or enforce")
	return c
}

func installProjectRulesHooks(root, providerName, mode string) error {
	if mode != "warn" && mode != "enforce" {
		return fmt.Errorf("mode must be warn or enforce")
	}
	switch providerName {
	case "codex":
		return installCodexProjectRulesHooks(root, mode)
	case "claude":
		return installClaudeProjectRulesHooks(root, mode)
	default:
		return fmt.Errorf("provider must be codex or claude")
	}
}

func installCodexProjectRulesHooks(root, mode string) error {
	hookRel := filepath.Join(".codex", "hooks", "project-rules-gate.ps1")
	hookPath := filepath.Join(root, hookRel)
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(hookPath, []byte(projectRulesHookScript(".codex/bin/codedungeon", mode)), 0o644); err != nil {
		return err
	}
	cfgPath := filepath.Join(root, ".codex", "config.toml")
	existing, _ := os.ReadFile(cfgPath)
	content := ensureCodexHooksFeature(string(existing))
	cmd := fmt.Sprintf(`powershell -NoProfile -ExecutionPolicy Bypass -File "$(git rev-parse --show-toplevel)/.codex/hooks/project-rules-gate.ps1" -Mode "%s"`, mode)
	block := fmt.Sprintf(`
# command delegates to codedungeon rules gate
[[hooks.UserPromptSubmit]]
[[hooks.UserPromptSubmit.hooks]]
type = "command"
command = '%s'
statusMessage = "Checking Project Rules"

[[hooks.PostToolUse]]
matcher = "Bash|apply_patch|Edit|Write"
[[hooks.PostToolUse.hooks]]
type = "command"
command = '%s'
statusMessage = "Checking Project Rules"

[[hooks.Stop]]
[[hooks.Stop.hooks]]
type = "command"
command = '%s'
timeout = 30
statusMessage = "Checking Project Rules completion gate"
`, cmd, cmd, cmd)
	content = upsertMarkerBlock(content, "# codedungeon project rules hooks", block)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(cfgPath, []byte(content), 0o644)
}

func installClaudeProjectRulesHooks(root, mode string) error {
	hookRel := filepath.Join(".claude", "hooks", "project-rules-gate.ps1")
	hookPath := filepath.Join(root, hookRel)
	if err := os.MkdirAll(filepath.Dir(hookPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(hookPath, []byte(projectRulesHookScript(".claude/bin/codedungeon", mode)), 0o644); err != nil {
		return err
	}
	settingsPath := filepath.Join(root, ".claude", "settings.json")
	settings := map[string]any{}
	if body, err := os.ReadFile(settingsPath); err == nil && len(strings.TrimSpace(string(body))) > 0 {
		_ = json.Unmarshal(body, &settings)
	}
	command := fmt.Sprintf(`powershell -NoProfile -ExecutionPolicy Bypass -File "$CLAUDE_PROJECT_DIR/.claude/hooks/project-rules-gate.ps1" -Mode "%s"`, mode)
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
	}
	for _, event := range []string{"UserPromptSubmit", "PostToolBatch", "TaskCreated", "TaskCompleted", "SubagentStop", "Stop"} {
		hooks[event] = []map[string]any{{
			"hooks": []map[string]any{{
				"type":    "command",
				"command": command,
				"timeout": 30,
			}},
		}}
	}
	settings["hooks"] = hooks
	settings["codedungeon_project_rules_hooks"] = map[string]any{
		"mode":    mode,
		"script":  ".claude/hooks/project-rules-gate.ps1",
		"command": fmt.Sprintf(".claude/bin/codedungeon rules gate --mode %s", mode),
	}
	body, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(settingsPath, body, 0o644)
}

func projectRulesHookScript(binaryRel, mode string) string {
	return fmt.Sprintf(`# codedungeon Project Rules hook adapter.
param([string]$Mode = "%s")
$ErrorActionPreference = "Stop"
$payload = [Console]::In.ReadToEnd()
$eventName = "unknown"
if (-not [string]::IsNullOrWhiteSpace($payload)) {
  try {
    $parsed = $payload | ConvertFrom-Json
    if ($parsed.hook_event_name) { $eventName = [string]$parsed.hook_event_name }
    elseif ($parsed.hookEventName) { $eventName = [string]$parsed.hookEventName }
  } catch {}
}
$root = (& git rev-parse --show-toplevel 2>$null)
if ([string]::IsNullOrWhiteSpace($root)) { $root = (Get-Location).Path }
$bin = Join-Path $root "%s"
if (!(Test-Path $bin) -and (Test-Path "$bin.exe")) { $bin = "$bin.exe" }
$mergePattern = 'gh\s+pr\s+merge|gh\s+api\b[^\r\n]*(/pulls/[^\s/]+/merge|/merge\b)|git(?:\s+-C\s+\S+)?\s+merge\b'
$mainPushPattern = 'git(?:\s+-C\s+\S+)?\s+push\b[^\r\n]*(origin\s+(main|HEAD:main|[^\s:]+:main)|origin[^\r\n]*refs/heads/main)'
$allowedReviewCommand = $payload -match 'codedungeon(\.exe)?\s+review\s+(run|post)\b'
$reviewPathMutation = ($payload -match '\.codedungeon[/\\]reviews') -and (-not $allowedReviewCommand)
if ($payload -match $mergePattern -or $payload -match $mainPushPattern -or $payload -match 'codedungeon\.db' -or $reviewPathMutation) {
  $active = $false
  try {
    $statusRaw = (& $bin run status 2>$null)
    if (-not [string]::IsNullOrWhiteSpace($statusRaw)) {
      $status = ($statusRaw | ConvertFrom-Json)
      if ($status.session -and $status.session.status -eq "RUNNING") { $active = $true }
    }
  } catch {}
  if ($active) {
    Write-Error "codedungeon autonomous session is active; direct merge/review/db mutation commands are blocked"
    exit 2
  }
}
& $bin rules gate --mode $Mode --event $eventName --message $payload
$gateCode = $LASTEXITCODE
if ($gateCode -ne 0 -and $Mode -eq "enforce" -and ($eventName -eq "Stop" -or $eventName -eq "SubagentStop")) {
  exit 2
}
exit $gateCode
`, mode, binaryRel)
}

func ensureCodexHooksFeature(content string) string {
	if strings.Contains(content, "codex_hooks = true") {
		return content
	}
	const features = "[features]"
	idx := strings.Index(content, features)
	if idx < 0 {
		if strings.TrimSpace(content) != "" && !strings.HasSuffix(content, "\n") {
			content += "\n"
		}
		return content + "\n[features]\ncodex_hooks = true\n"
	}
	insertAt := idx + len(features)
	return content[:insertAt] + "\ncodex_hooks = true" + content[insertAt:]
}

func upsertMarkerBlock(existing, marker, block string) string {
	start := marker + " begin"
	end := marker + " end"
	next := start + "\n" + strings.Trim(block, "\n") + "\n" + end + "\n"
	if strings.TrimSpace(existing) == "" {
		return next
	}
	idx := strings.Index(existing, start)
	if idx < 0 {
		if !strings.HasSuffix(existing, "\n") {
			existing += "\n"
		}
		return existing + "\n" + next
	}
	afterStart := existing[idx:]
	endIdx := strings.Index(afterStart, end)
	if endIdx < 0 {
		return strings.TrimRight(existing[:idx], "\n") + "\n" + next
	}
	endIdx += idx + len(end)
	return strings.TrimRight(existing[:idx], "\n") + "\n" + next + strings.TrimLeft(existing[endIdx:], "\n")
}
