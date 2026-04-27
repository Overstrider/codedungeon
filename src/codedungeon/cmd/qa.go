package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/tidwall/gjson"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/manifest"
	"github.com/loldinis/codedungeon/internal/osadapter"
)

func QACmd() *cobra.Command {
	c := &cobra.Command{Use: "qa", Short: "QA test helpers (API validation, framework detect)"}
	c.AddCommand(qaValidateAPICmd())
	c.AddCommand(qaDetectFrameworkCmd())
	c.AddCommand(qaRecordCmd())
	return c
}

func qaRecordCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "record",
		Short: "Record a concrete verification command in the phase ledger",
		RunE: func(c *cobra.Command, _ []string) error {
			phase, _ := c.Flags().GetString("phase")
			command, _ := c.Flags().GetString("cmd")
			status, _ := c.Flags().GetString("status")
			logPath, _ := c.Flags().GetString("log")
			if phase == "" {
				return EmitErr("--phase is required", "")
			}
			if command == "" {
				return EmitErr("--cmd is required", "")
			}
			status = strings.ToUpper(status)
			if status != "PASS" && status != "FAIL" {
				return EmitErr("--status must be PASS or FAIL", "")
			}
			if logPath == "" {
				return EmitErr("--log is required", "")
			}
			info, err := os.Stat(logPath)
			if err != nil {
				return EmitErr("verification log not found: "+logPath, "")
			}
			if info.Size() == 0 {
				return EmitErr("verification log is empty: "+logPath, "")
			}
			s, err := OpenDB(c)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			defer s.Close()
			run, err := s.CurrentRun()
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			if run == nil {
				return EmitErr("no active run", "run `codedungeon phase init` first")
			}
			id, err := s.InsertVerificationRecord(db.VerificationRecord{
				RunID:   run.ID,
				Phase:   phase,
				Command: command,
				Status:  status,
				LogPath: logPath,
			})
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			return EmitJSON(map[string]any{"ok": true, "id": id, "phase": phase, "status": status})
		},
	}
	c.Flags().String("phase", "", "phase number, usually 6")
	c.Flags().String("cmd", "", "verification command that was run")
	c.Flags().String("status", "", "PASS or FAIL")
	c.Flags().String("log", "", "path to non-empty command log")
	return c
}

// APISpec is the JSON input for `qa validate-api`.
type APISpec struct {
	Name    string            `json:"name,omitempty"`
	Method  string            `json:"method"`
	Path    string            `json:"path"`          // relative, appended to base-url
	URL     string            `json:"url,omitempty"` // alternative: full URL
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"` // string OR object (marshalled)
	Expect  struct {
		Status       int               `json:"status"`
		BodyContains []string          `json:"body_contains,omitempty"` // gjson paths
		BodyShape    map[string]string `json:"body_shape,omitempty"`    // path → type
		BodyAbsent   []string          `json:"body_absent,omitempty"`   // paths that must NOT exist
		BodyEqual    map[string]any    `json:"body_equal,omitempty"`    // path → expected value
	} `json:"expect"`
}

// ValidationResult is the JSON output.
type ValidationResult struct {
	OK      bool    `json:"ok"`
	Status  int     `json:"status"`
	TimeMs  int     `json:"time_ms"`
	Verdict string  `json:"verdict"` // PASS | FAIL
	Checks  []Check `json:"checks"`
	Body    string  `json:"body,omitempty"`
	Error   string  `json:"error,omitempty"`
}

type Check struct {
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail,omitempty"`
}

func qaValidateAPICmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "validate-api",
		Short: "Execute curl per --spec, validate status/body/shape/error-quality",
		RunE: func(c *cobra.Command, _ []string) error {
			specFile, _ := c.Flags().GetString("spec")
			baseURL, _ := c.Flags().GetString("base-url")
			tokenEnv, _ := c.Flags().GetString("token-env")
			if specFile == "" {
				return EmitErr("--spec <step.json> required", "")
			}
			raw, err := os.ReadFile(specFile)
			if err != nil {
				return EmitErr(err.Error(), "")
			}
			var spec APISpec
			if err := json.Unmarshal(raw, &spec); err != nil {
				return EmitErr(err.Error(), "invalid spec JSON")
			}

			url := spec.URL
			if url == "" {
				if baseURL == "" {
					return EmitErr("--base-url required when spec has no .url", "")
				}
				url = strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(spec.Path, "/")
			}

			// Token env substitution.
			token := ""
			if tokenEnv != "" {
				token = os.Getenv(tokenEnv)
			}

			headers := map[string]string{}
			for k, v := range spec.Headers {
				headers[k] = substEnv(v, map[string]string{"TOKEN": token})
			}

			result := runCurl(spec.Method, url, headers, spec.Body)
			res := ValidationResult{
				OK:     true,
				Status: result.Status,
				TimeMs: result.TimeMs,
				Body:   result.Body,
			}
			if result.Err != nil {
				res.Verdict = "FAIL"
				res.Error = result.Err.Error()
				return EmitJSON(res)
			}

			// Status check.
			res.Checks = append(res.Checks, Check{
				Name:   "status",
				Pass:   spec.Expect.Status == 0 || spec.Expect.Status == result.Status,
				Detail: fmt.Sprintf("want %d, got %d", spec.Expect.Status, result.Status),
			})

			// body_contains (keys / JSON paths).
			for _, p := range spec.Expect.BodyContains {
				res.Checks = append(res.Checks, checkContains(result.Body, p))
			}
			// body_absent.
			for _, p := range spec.Expect.BodyAbsent {
				res.Checks = append(res.Checks, checkAbsent(result.Body, p))
			}
			// body_shape.
			for p, typ := range spec.Expect.BodyShape {
				res.Checks = append(res.Checks, checkShape(result.Body, p, typ))
			}
			// body_equal.
			for p, v := range spec.Expect.BodyEqual {
				res.Checks = append(res.Checks, checkEqual(result.Body, p, v))
			}
			// error-quality for 4xx/5xx.
			if result.Status >= 400 {
				res.Checks = append(res.Checks, checkErrorQuality(result.Body))
			}

			// Overall verdict.
			res.Verdict = "PASS"
			for _, c := range res.Checks {
				if !c.Pass {
					res.Verdict = "FAIL"
					break
				}
			}
			return EmitJSON(res)
		},
	}
	c.Flags().String("spec", "", "path to step JSON")
	c.Flags().String("base-url", "", "base URL (required unless spec.url is set)")
	c.Flags().String("token-env", "", "env var holding bearer token (substituted for $TOKEN in headers)")
	return c
}

// ---- curl execution ----

type curlResult struct {
	Status int
	Body   string
	TimeMs int
	Err    error
}

func runCurl(method, url string, headers map[string]string, body any) curlResult {
	ad := osadapter.Detect()
	if method == "" {
		method = "GET"
	}
	args := []string{"-sS", "-X", method, url,
		"-o", "/tmp/_cd_body", "-w", "%{http_code}\n%{time_total}"}
	for k, v := range headers {
		args = append(args, "-H", k+": "+v)
	}
	if body != nil {
		b, _ := json.Marshal(body)
		// Default content-type if none.
		if _, hasCT := headers["Content-Type"]; !hasCT {
			args = append(args, "-H", "Content-Type: application/json")
		}
		args = append(args, "-d", string(b))
	}
	start := time.Now()
	out, errStr, err := ad.RunExec("", "curl", args...)
	elapsed := int(time.Since(start).Milliseconds())
	if err != nil {
		return curlResult{Err: fmt.Errorf("curl: %s (%w)", errStr, err), TimeMs: elapsed}
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	status := 0
	if len(lines) >= 1 {
		status, _ = strconv.Atoi(lines[0])
	}
	bodyBytes, _ := os.ReadFile("/tmp/_cd_body")
	return curlResult{Status: status, Body: string(bodyBytes), TimeMs: elapsed}
}

// ---- assertions ----

func checkContains(body, path string) Check {
	val := gjson.Get(body, path)
	if val.Exists() {
		return Check{Name: "body_contains:" + path, Pass: true}
	}
	return Check{Name: "body_contains:" + path, Pass: false, Detail: "missing"}
}

func checkAbsent(body, path string) Check {
	val := gjson.Get(body, path)
	if !val.Exists() {
		return Check{Name: "body_absent:" + path, Pass: true}
	}
	return Check{Name: "body_absent:" + path, Pass: false, Detail: "unexpectedly present"}
}

var (
	uuidRE    = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)
	iso8601RE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})?$`)
)

func checkShape(body, path, typ string) Check {
	val := gjson.Get(body, path)
	name := "body_shape:" + path + "=" + typ
	if !val.Exists() {
		return Check{Name: name, Pass: false, Detail: "missing"}
	}
	ok := false
	switch strings.ToLower(typ) {
	case "uuid":
		ok = uuidRE.MatchString(val.String())
	case "iso8601", "datetime":
		ok = iso8601RE.MatchString(val.String())
	case "string":
		ok = val.Type == gjson.String
	case "number", "int", "integer":
		ok = val.Type == gjson.Number
	case "boolean", "bool":
		ok = val.Type == gjson.True || val.Type == gjson.False
	case "array":
		ok = val.IsArray()
	case "object":
		ok = val.IsObject()
	default:
		ok = false
	}
	detail := ""
	if !ok {
		detail = fmt.Sprintf("got %s", val.Type.String())
	}
	return Check{Name: name, Pass: ok, Detail: detail}
}

func checkEqual(body, path string, want any) Check {
	got := gjson.Get(body, path)
	name := "body_equal:" + path
	if !got.Exists() {
		return Check{Name: name, Pass: false, Detail: "missing"}
	}
	wantJSON, _ := json.Marshal(want)
	if got.Raw == string(wantJSON) || got.String() == fmt.Sprint(want) {
		return Check{Name: name, Pass: true}
	}
	return Check{Name: name, Pass: false, Detail: fmt.Sprintf("want %v, got %s", want, got.String())}
}

var stackMarkers = []string{"Traceback", "panic:", "goroutine", "Exception in", "\n\tat "}

func checkErrorQuality(body string) Check {
	if strings.TrimSpace(body) == "" {
		return Check{Name: "error_quality", Pass: false, Detail: "empty body on 4xx/5xx"}
	}
	for _, m := range stackMarkers {
		if strings.Contains(body, m) {
			return Check{Name: "error_quality", Pass: false, Detail: "leaks stack trace: " + m}
		}
	}
	return Check{Name: "error_quality", Pass: true}
}

var envRE = regexp.MustCompile(`\$(\w+)`)

func substEnv(s string, extra map[string]string) string {
	return envRE.ReplaceAllStringFunc(s, func(m string) string {
		name := strings.TrimPrefix(m, "$")
		if v, ok := extra[name]; ok && v != "" {
			return v
		}
		if v := os.Getenv(name); v != "" {
			return v
		}
		return m
	})
}

// ---- detect-framework ----

func qaDetectFrameworkCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "detect-framework",
		Short: "Detect primary test framework from manifest + file conventions",
		RunE: func(c *cobra.Command, _ []string) error {
			path, _ := c.Flags().GetString("path")
			if path == "" {
				path = "."
			}
			info, _ := manifest.Detect(path)
			fw, cfg, cmd := detectTestFramework(path, info.Lang)
			return EmitJSON(map[string]any{
				"ok":        true,
				"path":      path,
				"lang":      info.Lang,
				"framework": fw,
				"config":    cfg,
				"run_cmd":   cmd,
			})
		},
	}
	c.Flags().String("path", ".", "project dir to inspect")
	return c
}

// detectTestFramework returns (framework, config_file, run_command) for the lang.
func detectTestFramework(path, lang string) (string, string, string) {
	existsAt := func(p string) bool {
		_, err := os.Stat(filepath.Join(path, p))
		return err == nil
	}
	pkgJSON := func(key string) bool {
		b, err := os.ReadFile(filepath.Join(path, "package.json"))
		if err != nil {
			return false
		}
		raw := string(b)
		return strings.Contains(raw, `"`+key+`"`)
	}
	switch lang {
	case "python":
		if existsAt("pytest.ini") || pyprojectContains(path, "[tool.pytest") {
			return "pytest", "pytest.ini", "pytest"
		}
		return "unittest", "", "python -m unittest"
	case "nextjs", "react", "vue", "typescript":
		switch {
		case pkgJSON("playwright"):
			return "playwright", detectFirstExisting(path, "playwright.config.ts", "playwright.config.js"), "npx playwright test"
		case pkgJSON("vitest"):
			return "vitest", detectFirstExisting(path, "vitest.config.ts", "vitest.config.js"), "npx vitest run"
		case pkgJSON("jest"):
			return "jest", detectFirstExisting(path, "jest.config.ts", "jest.config.js", "jest.config.json"), "npx jest"
		case pkgJSON("mocha"):
			return "mocha", "", "npx mocha"
		}
		return "unknown", "", ""
	case "go":
		return "go test", "", "go test ./..."
	case "rust":
		return "cargo test", "Cargo.toml", "cargo test"
	case "kotlin":
		return "junit", "build.gradle.kts", "./gradlew test"
	case "elixir":
		return "exunit", "mix.exs", "mix test"
	case "cpp":
		return "ctest", "CMakeLists.txt", "ctest"
	case "php":
		return "phpunit", "phpunit.xml", "vendor/bin/phpunit"
	case "ruby":
		return "rspec", "Gemfile", "bundle exec rspec"
	}
	return "unknown", "", ""
}

func pyprojectContains(path, needle string) bool {
	b, err := os.ReadFile(filepath.Join(path, "pyproject.toml"))
	if err != nil {
		return false
	}
	return strings.Contains(string(b), needle)
}

func detectFirstExisting(root string, names ...string) string {
	for _, n := range names {
		p := filepath.Join(root, n)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}
