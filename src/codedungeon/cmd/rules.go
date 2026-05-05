package cmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	projectRulesRel        = ".codedungeon/project-rules.md"
	projectRulesCompactRel = ".codedungeon/project-rules.compact.md"
	projectRulesStateRel   = ".codedungeon/project-rules.json"
)

type projectRulesState struct {
	Status       string   `json:"status"`
	RulesPath    string   `json:"rules_path"`
	CompactPath  string   `json:"compact_path"`
	SourceDigest string   `json:"source_digest"`
	RulesDigest  string   `json:"rules_digest"`
	ApprovedAt   string   `json:"approved_at,omitempty"`
	ApprovedBy   string   `json:"approved_by,omitempty"`
	Sources      []string `json:"sources"`
}

type projectRulesStatus struct {
	OK           bool     `json:"ok"`
	Status       string   `json:"status"`
	RulesPath    string   `json:"rules_path"`
	CompactPath  string   `json:"compact_path"`
	StatePath    string   `json:"state_path"`
	SourceDigest string   `json:"source_digest"`
	RulesDigest  string   `json:"rules_digest"`
	StaleReason  string   `json:"stale_reason,omitempty"`
	Missing      []string `json:"missing,omitempty"`
	Sources      []string `json:"sources,omitempty"`
}

type lintResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

type gateOptions struct {
	Event   string
	Mode    string
	Message string
}

type gateResult struct {
	OK       bool   `json:"ok"`
	Mode     string `json:"mode"`
	Event    string `json:"event"`
	Status   string `json:"project_rules_status"`
	Message  string `json:"message"`
	Continue bool   `json:"continue,omitempty"`
}

func RulesCmd() *cobra.Command {
	c := &cobra.Command{Use: "rules", Short: "Manage project rules discovery state"}
	c.AddCommand(rulesStatusCmd())
	c.AddCommand(rulesLintCmd())
	c.AddCommand(rulesDigestCmd())
	c.AddCommand(rulesApproveCmd())
	c.AddCommand(rulesCompactCmd())
	c.AddCommand(rulesGateCmd())
	return c
}

func rulesStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Print project rules status and digests",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			st, err := computeProjectRulesStatus(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(st)
		},
	}
}

func rulesLintCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "lint",
		Short: "Validate project rules files",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			res := lintProjectRules(root)
			if !res.OK {
				_ = EmitJSON(res)
				return fmt.Errorf("project rules lint failed")
			}
			return EmitJSON(res)
		},
	}
}

func rulesDigestCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "digest",
		Short: "Recompute project rules source and rules digests",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			st, err := computeProjectRulesStatus(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(st)
		},
	}
}

func rulesApproveCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "approve",
		Short: "Mark a reviewed Project Rules draft as approved",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			by, _ := c.Flags().GetString("by")
			if by == "" {
				by = os.Getenv("USER")
				if by == "" {
					by = os.Getenv("USERNAME")
				}
				if by == "" {
					by = "unknown"
				}
			}
			st, err := approveProjectRules(root, by)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(st)
		},
	}
	c.Flags().String("by", "", "approver name for project-rules.json")
	return c
}

func rulesCompactCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "compact",
		Short: "Generate compact operational Project Rules from approved rules",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			st, err := compactProjectRules(root)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(st)
		},
	}
}

func rulesGateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "gate",
		Short: "Evaluate Project Rules hook gates",
		RunE: func(c *cobra.Command, _ []string) error {
			root := projectRulesRoot()
			event, _ := c.Flags().GetString("event")
			mode, _ := c.Flags().GetString("mode")
			message, _ := c.Flags().GetString("message")
			res := evaluateRulesGate(root, gateOptions{Event: event, Mode: mode, Message: message})
			if !res.OK && mode == "enforce" {
				_ = EmitJSON(res)
				return fmt.Errorf("%s", res.Message)
			}
			return EmitJSON(res)
		},
	}
	c.Flags().String("event", "", "provider hook event name")
	c.Flags().String("mode", "warn", "warn or enforce")
	c.Flags().String("message", "", "optional final message or hook payload summary")
	return c
}

func computeProjectRulesStatus(root string) (projectRulesStatus, error) {
	rulesPath := filepath.Join(root, filepath.FromSlash(projectRulesRel))
	compactPath := filepath.Join(root, filepath.FromSlash(projectRulesCompactRel))
	statePath := filepath.Join(root, filepath.FromSlash(projectRulesStateRel))
	status := projectRulesStatus{
		OK:          true,
		Status:      "missing",
		RulesPath:   projectRulesRel,
		CompactPath: projectRulesCompactRel,
		StatePath:   projectRulesStateRel,
	}

	var missing []string
	rulesBody, rulesErr := os.ReadFile(rulesPath)
	if rulesErr != nil {
		missing = append(missing, projectRulesRel)
	}
	if _, err := os.Stat(compactPath); err != nil {
		missing = append(missing, projectRulesCompactRel)
	}
	if _, err := os.Stat(statePath); err != nil {
		missing = append(missing, projectRulesStateRel)
	}
	status.Missing = missing

	sourceDigest, sources, err := computeProjectRulesSourceDigest(root)
	if err != nil {
		return status, err
	}
	status.SourceDigest = sourceDigest
	status.Sources = sources

	if rulesErr != nil {
		status.OK = false
		return status, nil
	}
	rulesDigest := digestBytes(rulesBody)
	status.RulesDigest = rulesDigest
	rulesTextStatus := parseRulesTextStatus(string(rulesBody))
	if rulesTextStatus == "draft" {
		status.Status = "draft"
		return status, nil
	}

	state, err := readProjectRulesState(statePath)
	if err != nil {
		if rulesTextStatus == "approved" && len(missing) == 0 {
			status.Status = "approved"
		}
		return status, nil
	}
	status.Status = state.Status
	if state.RulesDigest != rulesDigest {
		status.Status = "stale"
		status.StaleReason = "rules digest changed"
	}
	if state.SourceDigest != sourceDigest {
		status.Status = "stale"
		status.StaleReason = "source digest changed"
	}
	status.OK = status.Status == "approved"
	if status.Status == "missing" || status.Status == "" {
		status.OK = false
	}
	return status, nil
}

func approveProjectRules(root, approvedBy string) (projectRulesStatus, error) {
	rulesPath := filepath.Join(root, filepath.FromSlash(projectRulesRel))
	body, err := os.ReadFile(rulesPath)
	if err != nil {
		return projectRulesStatus{}, err
	}
	next := setRulesTextStatus(string(body), "APPROVED")
	if err := os.WriteFile(rulesPath, []byte(next), 0o644); err != nil {
		return projectRulesStatus{}, err
	}
	sourceDigest, sources, err := computeProjectRulesSourceDigest(root)
	if err != nil {
		return projectRulesStatus{}, err
	}
	state := projectRulesState{
		Status:       "approved",
		RulesPath:    projectRulesRel,
		CompactPath:  projectRulesCompactRel,
		SourceDigest: sourceDigest,
		RulesDigest:  digestBytes([]byte(next)),
		ApprovedAt:   time.Now().UTC().Format(time.RFC3339),
		ApprovedBy:   approvedBy,
		Sources:      sources,
	}
	if err := writeProjectRulesState(root, state); err != nil {
		return projectRulesStatus{}, err
	}
	return computeProjectRulesStatus(root)
}

func compactProjectRules(root string) (projectRulesStatus, error) {
	rulesPath := filepath.Join(root, filepath.FromSlash(projectRulesRel))
	body, err := os.ReadFile(rulesPath)
	if err != nil {
		return projectRulesStatus{}, err
	}
	if parseRulesTextStatus(string(body)) != "approved" {
		return projectRulesStatus{}, fmt.Errorf("project rules must be approved before compacting")
	}
	bullets := extractOperationalBullets(string(body))
	if len(bullets) == 0 {
		return projectRulesStatus{}, fmt.Errorf("no operational bullets found in approved project rules")
	}
	if len(bullets) > 20 {
		bullets = bullets[:20]
	}
	var out strings.Builder
	out.WriteString("# Project Rules Compact\n\n")
	out.WriteString("PROJECT_RULES_STATUS: APPROVED\n")
	out.WriteString("PROJECT_RULES_SOURCE: .codedungeon/project-rules.md\n\n")
	for _, b := range bullets {
		out.WriteString(b)
		out.WriteString("\n")
	}
	compactPath := filepath.Join(root, filepath.FromSlash(projectRulesCompactRel))
	if err := os.MkdirAll(filepath.Dir(compactPath), 0o755); err != nil {
		return projectRulesStatus{}, err
	}
	if err := os.WriteFile(compactPath, []byte(out.String()), 0o644); err != nil {
		return projectRulesStatus{}, err
	}
	return computeProjectRulesStatus(root)
}

func lintProjectRules(root string) lintResult {
	res := lintResult{OK: true}
	rulesPath := filepath.Join(root, filepath.FromSlash(projectRulesRel))
	compactPath := filepath.Join(root, filepath.FromSlash(projectRulesCompactRel))
	bodyBytes, err := os.ReadFile(rulesPath)
	if err != nil {
		res.OK = false
		res.Errors = append(res.Errors, "missing .codedungeon/project-rules.md")
		return res
	}
	body := string(bodyBytes)
	requiredSections := []string{
		"## Sources Reviewed",
		"## Architecture And Boundaries",
		"## Project Rules",
		"## Commands And Verification",
		"## Security And Data Rules",
		"## Agent Operating Rules",
		"## Open Questions",
	}
	for _, section := range requiredSections {
		if !strings.Contains(body, section) {
			res.Errors = append(res.Errors, "missing section "+section)
		}
	}
	if parseRulesTextStatus(body) == "approved" && hasUnresolvedOpenQuestions(body) {
		res.Errors = append(res.Errors, "approved rules must not have unresolved open questions")
	}
	compact, err := os.ReadFile(compactPath)
	if err != nil {
		res.Errors = append(res.Errors, "missing .codedungeon/project-rules.compact.md")
	} else {
		compactBody := string(compact)
		if !strings.Contains(compactBody, "PROJECT_RULES_STATUS: APPROVED") {
			res.Errors = append(res.Errors, "compact rules must be approved")
		}
		var bullets int
		for _, line := range strings.Split(compactBody, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				bullets++
				if !hasRuleVerb(line) {
					res.Errors = append(res.Errors, "compact bullet must start with MUST, MUST NOT, VERIFY, or ASK WHEN")
					break
				}
			}
		}
		if bullets > 20 {
			res.Errors = append(res.Errors, "compact rules must have at most 20 bullets")
		}
	}
	if len(res.Errors) > 0 {
		res.OK = false
	}
	return res
}

func evaluateRulesGate(root string, opts gateOptions) gateResult {
	if opts.Mode == "" {
		opts.Mode = "warn"
	}
	st, err := computeProjectRulesStatus(root)
	if err != nil {
		return gateResult{OK: false, Mode: opts.Mode, Event: opts.Event, Message: err.Error()}
	}
	res := gateResult{OK: true, Mode: opts.Mode, Event: opts.Event, Status: st.Status, Message: "project rules gate passed"}
	claimsComplete := strings.Contains(strings.ToLower(opts.Message), "complete")
	if st.Status != "approved" {
		res.OK = opts.Mode != "enforce"
		res.Message = fmt.Sprintf("PROJECT_RULES_STATUS: %s - run `$codedungeon --rules` or `/codedungeon --rules` and approve rules before completion", st.Status)
		res.Continue = opts.Mode == "enforce" && strings.EqualFold(opts.Event, "Stop") && claimsComplete
		return res
	}
	if strings.EqualFold(opts.Event, "Stop") && claimsComplete {
		for _, required := range []string{"PROJECT_RULES_STATUS", "PROJECT_RULES_DIGEST", "Verification: PASS"} {
			if !strings.Contains(opts.Message, required) {
				res.OK = opts.Mode != "enforce"
				res.Message = "completion claim missing " + required
				res.Continue = opts.Mode == "enforce"
				return res
			}
		}
	}
	return res
}

func computeProjectRulesSourceDigest(root string) (string, []string, error) {
	ignore := loadGitIgnore(root)
	var sources []string
	allowedIgnoredDirs := map[string]bool{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == "." {
				return nil
			}
			if isGeneratedRulesPath(rel) {
				return filepath.SkipDir
			}
			if ignore.matches(rel, true) && !hasAllowedIgnoredAncestor(rel, allowedIgnoredDirs) {
				if isProjectRulesBoundaryDir(root, rel) {
					allowedIgnoredDirs[rel] = true
					return nil
				}
				return filepath.SkipDir
			}
			return nil
		}
		if isGeneratedRulesPath(rel) || !isProjectRulesSource(rel) {
			return nil
		}
		if ignore.matches(rel, false) && !hasAllowedIgnoredAncestor(rel, allowedIgnoredDirs) {
			return nil
		}
		sources = append(sources, rel)
		return nil
	})
	if err != nil {
		return "", nil, err
	}
	sort.Strings(sources)
	h := sha256.New()
	for _, rel := range sources {
		b, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", nil, err
		}
		h.Write([]byte(rel))
		h.Write([]byte{0})
		h.Write(b)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), sources, nil
}

func isGeneratedRulesPath(rel string) bool {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for _, part := range parts {
		switch part {
		case ".git", ".codedungeon", ".codex", ".claude", ".agents", "node_modules", "target", "dist", "build", ".next", "coverage", ".cache":
			return true
		}
	}
	return false
}

type gitIgnorePatterns []string

func loadGitIgnore(root string) gitIgnorePatterns {
	var patterns []string
	for _, name := range []string{".gitignore", filepath.Join(".git", "info", "exclude")} {
		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			continue
		}
		patterns = append(patterns, parseIgnorePatterns(string(body))...)
	}
	return patterns
}

func parseIgnorePatterns(body string) gitIgnorePatterns {
	var patterns []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		patterns = append(patterns, filepath.ToSlash(strings.TrimPrefix(line, "/")))
	}
	return patterns
}

func (patterns gitIgnorePatterns) matches(rel string, isDir bool) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	for _, pattern := range patterns {
		dirOnly := strings.HasSuffix(pattern, "/")
		clean := strings.TrimSuffix(pattern, "/")
		if clean == "" || (dirOnly && !isDir && !strings.HasPrefix(rel, clean+"/")) {
			continue
		}
		if strings.Contains(clean, "/") {
			if rel == clean || strings.HasPrefix(rel, clean+"/") {
				return true
			}
			continue
		}
		for _, part := range strings.Split(rel, "/") {
			if part == clean {
				return true
			}
		}
	}
	return false
}

func isProjectRulesSource(rel string) bool {
	base := filepath.Base(rel)
	switch base {
	case "README.md", "AGENTS.md", "CLAUDE.md", "go.mod", "package.json", "pnpm-lock.yaml",
		"Cargo.toml", "pyproject.toml", "Dockerfile", "Containerfile", ".env.example",
		"docker-compose.yml", "docker-compose.yaml", "Makefile":
		return true
	}
	if strings.HasPrefix(rel, "docs/") && strings.HasSuffix(rel, ".md") {
		return true
	}
	if strings.HasPrefix(rel, ".github/workflows/") {
		return true
	}
	if strings.HasSuffix(base, ".config.js") || strings.HasSuffix(base, ".config.ts") || strings.HasSuffix(base, ".toml") {
		return true
	}
	return false
}

func isProjectRulesBoundaryDir(root, rel string) bool {
	for _, name := range []string{
		"AGENTS.md",
		"CLAUDE.md",
		"README.md",
		"go.mod",
		"package.json",
		"Cargo.toml",
		"pyproject.toml",
		"build.gradle.kts",
		filepath.Join("gradle", "libs.versions.toml"),
		"Dockerfile",
		"Containerfile",
	} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel), name)); err == nil {
			return true
		}
	}
	return false
}

func hasAllowedIgnoredAncestor(rel string, allowed map[string]bool) bool {
	for dir := filepath.ToSlash(filepath.Dir(rel)); dir != "." && dir != ""; dir = filepath.ToSlash(filepath.Dir(dir)) {
		if allowed[dir] {
			return true
		}
	}
	return false
}

func digestBytes(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

func readProjectRulesState(path string) (projectRulesState, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return projectRulesState{}, err
	}
	var state projectRulesState
	if err := json.Unmarshal(body, &state); err != nil {
		return projectRulesState{}, err
	}
	return state, nil
}

func writeProjectRulesState(root string, state projectRulesState) error {
	path := filepath.Join(root, filepath.FromSlash(projectRulesStateRel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(path, body, 0o644)
}

func parseRulesTextStatus(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "status:") {
			return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "Status:")))
		}
	}
	return "missing"
}

func setRulesTextStatus(body, status string) string {
	lines := strings.Split(body, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "status:") {
			lines[i] = "Status: " + status
			return strings.Join(lines, "\n")
		}
	}
	return strings.TrimRight(body, "\n") + "\n\nStatus: " + status + "\n"
}

func extractOperationalBullets(body string) []string {
	seen := map[string]bool{}
	var bullets []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") || !hasRuleVerb(line) {
			continue
		}
		if seen[line] {
			continue
		}
		seen[line] = true
		bullets = append(bullets, line)
	}
	return bullets
}

func hasRuleVerb(line string) bool {
	text := strings.TrimSpace(strings.TrimPrefix(line, "- "))
	return strings.HasPrefix(text, "MUST ") ||
		strings.HasPrefix(text, "MUST NOT ") ||
		strings.HasPrefix(text, "VERIFY ") ||
		strings.HasPrefix(text, "ASK WHEN ")
}

func hasUnresolvedOpenQuestions(body string) bool {
	section := sectionBody(body, "## Open Questions")
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
		if line == "" {
			continue
		}
		lower := strings.ToLower(strings.Trim(line, ". "))
		if lower == "none" || lower == "n/a" || lower == "no open questions" {
			continue
		}
		return true
	}
	return false
}

func projectRulesRoot() string {
	return ResolveCodeDungeonRoot(mustGetwd())
}

func sectionBody(body, header string) string {
	idx := strings.Index(body, header)
	if idx < 0 {
		return ""
	}
	rest := body[idx+len(header):]
	next := strings.Index(rest, "\n## ")
	if next >= 0 {
		rest = rest[:next]
	}
	return rest
}

func mustGetwd() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}
