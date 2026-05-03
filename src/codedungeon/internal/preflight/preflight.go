package preflight

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/qa"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

func Run(ctx context.Context, req Request) (Report, error) {
	if strings.TrimSpace(req.Root) == "" {
		cwd, _ := os.Getwd()
		req.Root = cwd
	}
	root, err := filepath.Abs(req.Root)
	if err != nil {
		return Report{}, err
	}
	req.Root = root
	if req.Provider == nil {
		req.Provider = provider.Detect()
	}
	if req.Runner == nil {
		req.Runner = tooladapter.NewSystemRunner()
	}
	if req.FileSystem == nil {
		req.FileSystem = tooladapter.OSFileSystem{}
	}

	report := Report{OK: true, Root: root, Provider: req.Provider.Name()}
	add := func(id, status, detail string, blocker bool, next ...string) {
		report.Checks = append(report.Checks, Check{ID: id, Status: status, Detail: detail, Blocker: blocker})
		if blocker || (req.Strict && status == StatusWarn) {
			report.OK = false
		}
		if blocker {
			report.NextCommands = append(report.NextCommands, next...)
		}
	}

	checkHomeConfig(req, add)
	checkGitRepository(req, add)
	checkGitCLI(ctx, req, add)
	checkGitState(ctx, req, add)
	checkGitHub(ctx, req, add)
	checkProviderCLI(ctx, req, add)
	store := checkDatabase(req, add)
	checkModels(store, add)
	if store != nil {
		defer store.Close()
	}
	checkProjectContext(req, add)
	checkPlanningArtifacts(req, add)
	checkSandbox(req, add)
	checkSecrets(req, add)
	checkFramework(req, add)
	checkPorts(add)

	report.NextCommands = dedupe(report.NextCommands)
	return report, nil
}

func checkHomeConfig(req Request, add func(string, string, string, bool, ...string)) {
	if isHomeConfig(req.Root, req.Provider) {
		add("home_config", StatusFail, "root is under provider home config directory", true, "cd <project>")
		return
	}
	add("home_config", StatusPass, "project directory is outside provider home config", false)
}

func checkGitRepository(req Request, add func(string, string, string, bool, ...string)) {
	if _, err := req.FileSystem.Stat(filepath.Join(req.Root, ".git")); err != nil {
		add("git", StatusFail, ".git not found", true, "git init")
		return
	}
	add("git", StatusPass, "git repository found", false)
}

func checkGitCLI(ctx context.Context, req Request, add func(string, string, string, bool, ...string)) {
	result, err := req.Runner.Run(ctx, tooladapter.Command{Dir: req.Root, Name: "git", Args: []string{"--version"}, Timeout: 10 * time.Second})
	if err != nil {
		add("git_cli", StatusFail, err.Error(), true, "install git")
		return
	}
	add("git_cli", StatusPass, strings.TrimSpace(result.Stdout), false)
}

func checkGitState(ctx context.Context, req Request, add func(string, string, string, bool, ...string)) {
	git := tooladapter.NewGitClient(req.Runner)
	branch, err := git.CurrentBranch(ctx, req.Root)
	if err != nil {
		add("git_branch", StatusWarn, err.Error(), false)
	} else if branch == "" {
		add("git_branch", StatusWarn, "detached HEAD or branch unavailable", false)
	} else if protectedBranches[branch] {
		add("git_branch", StatusWarn, "current branch is protected: "+branch, false)
	} else {
		add("git_branch", StatusPass, branch, false)
	}
	origin, err := git.RemoteOrigin(ctx, req.Root)
	if err != nil || strings.TrimSpace(origin) == "" {
		add("git_origin", StatusWarn, "origin remote not configured", false)
		return
	}
	add("git_origin", StatusPass, origin, false)
}

func checkGitHub(ctx context.Context, req Request, add func(string, string, string, bool, ...string)) {
	if result, err := req.Runner.Run(ctx, tooladapter.Command{Dir: req.Root, Name: "gh", Args: []string{"--version"}, Timeout: 10 * time.Second}); err != nil {
		add("gh_cli", StatusFail, err.Error(), true, "install gh")
		return
	} else {
		add("gh_cli", StatusPass, firstLine(result.Stdout), false)
	}
	gh := tooladapter.NewGitHubClient(req.Runner)
	if err := gh.AuthStatus(ctx, req.Root); err != nil {
		add("gh_auth", StatusFail, err.Error(), true, "gh auth login")
		return
	}
	add("gh_auth", StatusPass, "authenticated", false)
	repo, err := gh.RepoName(ctx, req.Root)
	if err != nil {
		add("gh_repo", StatusWarn, err.Error(), false)
		return
	}
	add("gh_repo", StatusPass, repo, false)
}

func checkProviderCLI(ctx context.Context, req Request, add func(string, string, string, bool, ...string)) {
	name := req.Provider.Name()
	result, err := req.Runner.Run(ctx, tooladapter.Command{Dir: req.Root, Name: name, Args: []string{"--version"}, Timeout: 10 * time.Second})
	if err != nil {
		add("provider_cli", StatusFail, err.Error(), true, "install "+name)
		return
	}
	add("provider_cli", StatusPass, firstLine(result.Stdout), false)
}

func checkDatabase(req Request, add func(string, string, string, bool, ...string)) *db.Store {
	dbPath := filepath.Join(req.Root, req.Provider.DBPath())
	if _, err := req.FileSystem.Stat(dbPath); err != nil {
		add("database", StatusFail, dbPath+" not found", true, "codedungeon phase init --feature <feature> --branch <branch> --project-mode SINGLE")
		return nil
	}
	store, err := db.Open(dbPath)
	if err != nil {
		add("database", StatusFail, err.Error(), true, "codedungeon migrate")
		return nil
	}
	if err := store.Init(); err != nil {
		add("database", StatusFail, err.Error(), true, "codedungeon migrate")
		_ = store.Close()
		return nil
	}
	add("database", StatusPass, dbPath, false)
	return store
}

func checkModels(store *db.Store, add func(string, string, string, bool, ...string)) {
	if store == nil {
		add("models", StatusFail, "database unavailable", true, "codedungeon config set-models")
		return
	}
	reasoning, _ := store.GetMeta("model_reasoning")
	fast, _ := store.GetMeta("model_fast")
	var missing []string
	if strings.TrimSpace(reasoning) == "" {
		missing = append(missing, "model_reasoning")
	}
	if strings.TrimSpace(fast) == "" {
		missing = append(missing, "model_fast")
	}
	if len(missing) > 0 {
		add("models", StatusFail, "missing "+strings.Join(missing, ", "), true, "codedungeon config set-models")
		return
	}
	add("models", StatusPass, fmt.Sprintf("reasoning=%s fast=%s", reasoning, fast), false)
}

func checkProjectContext(req Request, add func(string, string, string, bool, ...string)) {
	if firstExisting(req.FileSystem,
		filepath.Join(req.Root, ".codedungeon", "project-context.md"),
		filepath.Join(req.Root, ".codedungeon", "project-rules.compact.md"),
	) == "" {
		add("project_context", StatusFail, "no project context file found", true, "codedungeon project-context build")
		return
	}
	add("project_context", StatusPass, "project context found", false)
}

func checkPlanningArtifacts(req Request, add func(string, string, string, bool, ...string)) {
	planPath := filepath.Join(req.Root, ".codedungeon", "plan", "PLAN.md")
	taskMatches, _ := req.FileSystem.Glob(filepath.Join(req.Root, ".codedungeon", "tasks", "task-*.md"))
	if _, err := req.FileSystem.Stat(planPath); err != nil || len(taskMatches) == 0 {
		add("planning_artifacts", StatusFail, "canonical PLAN.md or task-*.md files missing", true, "codedungeon plan run --prompt <task> --promote --auto-repair")
		return
	}
	add("planning_artifacts", StatusPass, planPath, false)
}

func checkSandbox(req Request, add func(string, string, string, bool, ...string)) {
	if req.Provider.Name() == "codex" && truthy(os.Getenv("CODEX_SANDBOX_NETWORK_DISABLED")) {
		add("sandbox", StatusFail, "CODEX_SANDBOX_NETWORK_DISABLED blocks nested Codex agents", true, "run outside Codex sandbox or use --runner files")
		return
	}
	add("sandbox", StatusPass, "no blocking sandbox signal detected", false)
}

func checkSecrets(req Request, add func(string, string, string, bool, ...string)) {
	findings := scanSecrets(req.Root)
	if len(findings) == 0 {
		add("secrets", StatusPass, "no committed secret-like assignments found", false)
		return
	}
	sort.Strings(findings)
	add("secrets", StatusFail, strings.Join(findings, "; "), true, "remove committed secrets and rotate exposed keys")
}

func checkFramework(req Request, add func(string, string, string, bool, ...string)) {
	result := qa.DetectFramework(req.Root)
	if result.Lang == "unknown" || result.Framework == "unknown" {
		add("framework", StatusWarn, "framework not detected", false)
		return
	}
	detail := result.Lang
	if result.Framework != "" {
		detail += "/" + result.Framework
	}
	if len(result.RunCommands) > 0 {
		detail += " run=" + strings.Join(result.RunCommands, " && ")
	}
	add("framework", StatusPass, detail, false)
}

func checkPorts(add func(string, string, string, bool, ...string)) {
	var occupied []string
	for _, name := range []string{"PORT", "APP_PORT", "API_PORT", "FRONTEND_PORT", "BACKEND_PORT"} {
		value := strings.TrimSpace(os.Getenv(name))
		if value == "" {
			continue
		}
		if !portRE.MatchString(value) {
			continue
		}
		ln, err := net.Listen("tcp", "127.0.0.1:"+value)
		if err != nil {
			occupied = append(occupied, name+"="+value)
			continue
		}
		_ = ln.Close()
	}
	if len(occupied) > 0 {
		add("ports", StatusWarn, "ports already in use: "+strings.Join(occupied, ", "), false)
		return
	}
	add("ports", StatusPass, "no configured port conflicts detected", false)
}

func isHomeConfig(path string, p provider.Provider) bool {
	abs, _ := filepath.Abs(path)
	abs = filepath.Clean(abs)
	for _, guard := range p.HomeGuardPaths() {
		guardAbs, _ := filepath.Abs(filepath.FromSlash(guard))
		guardAbs = filepath.Clean(guardAbs)
		if abs == guardAbs || strings.HasPrefix(abs, guardAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func firstExisting(fs tooladapter.FileSystem, paths ...string) string {
	for _, path := range paths {
		if _, err := fs.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

func scanSecrets(root string) []string {
	var findings []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			if shouldSkipDir(entry.Name()) && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil || info.Size() > 512*1024 {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil || strings.IndexByte(string(body), 0) >= 0 {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		lines := strings.Split(string(body), "\n")
		for i, line := range lines {
			if match := secretAssignRE.FindStringSubmatch(line); len(match) == 3 && isRealSecret(match[2]) {
				findings = append(findings, fmt.Sprintf("%s:%d %s=%s", filepath.ToSlash(rel), i+1, strings.ToUpper(match[1]), redact(match[2])))
			}
		}
		return nil
	})
	return findings
}

func shouldSkipDir(name string) bool {
	switch name {
	case ".git", ".codedungeon", ".codex", ".claude", ".agents", "node_modules", "target", "dist", "build", ".next", "coverage", ".cache":
		return true
	default:
		return false
	}
}

func isRealSecret(value string) bool {
	value = strings.Trim(value, `"'`)
	if strings.Contains(strings.ToLower(value), "example") || strings.Contains(strings.ToLower(value), "placeholder") {
		return false
	}
	return len(value) >= 16
}

func redact(value string) string {
	value = strings.TrimSpace(strings.Trim(value, `"'`))
	if len(value) <= 8 {
		return value[:2] + "..."
	}
	prefix := 6
	if strings.HasPrefix(value, "sk-proj") {
		prefix = 7
	}
	if len(value) < prefix+4 {
		return value[:prefix] + "..."
	}
	return value[:prefix] + "..." + value[len(value)-4:]
}

func firstLine(value string) string {
	value = strings.TrimSpace(value)
	if idx := strings.IndexByte(value, '\n'); idx >= 0 {
		return strings.TrimSpace(value[:idx])
	}
	return value
}

func truthy(value string) bool {
	value = strings.TrimSpace(value)
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes")
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

var (
	protectedBranches = map[string]bool{"main": true, "master": true, "develop": true, "dev": true, "staging": true, "production": true, "release": true}
	portRE            = regexp.MustCompile(`^[0-9]{2,5}$`)
	secretAssignRE    = regexp.MustCompile(`(?i)\b(OPENROUTER_API_KEY|OPENAI_API_KEY|ANTHROPIC_API_KEY|GITHUB_TOKEN|GH_TOKEN)\b\s*=\s*["']?([A-Za-z0-9_\-]{16,})`)
)
