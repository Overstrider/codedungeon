package cmd

import (
	"path/filepath"
	"testing"

	"github.com/loldinis/codedungeon/internal/provider"
)

func TestParsePlanMD(t *testing.T) {
	src := `# Feature: add foo
# Repo: backend
# Lang: go

## Tasks
- [x] TASK-001 seed
- [x] TASK-002 add middleware
- [ ] TASK-003 handler
- [!] TASK-004 blocked
- [ ] TASK-005 tests
`
	m := parsePlanMD(src)
	if m.Feature != "add foo" {
		t.Errorf("feature = %q", m.Feature)
	}
	if m.Repo != "backend" {
		t.Errorf("repo = %q", m.Repo)
	}
	if m.Lang != "go" {
		t.Errorf("lang = %q", m.Lang)
	}
	if m.TotalTasks != 5 {
		t.Errorf("total = %d", m.TotalTasks)
	}
	if m.Done != 2 {
		t.Errorf("done = %d", m.Done)
	}
	if m.Pending != 2 {
		t.Errorf("pending = %d", m.Pending)
	}
	if m.Blocked != 1 {
		t.Errorf("blocked = %d", m.Blocked)
	}
	if m.MaxTaskNum != 5 {
		t.Errorf("max = %d", m.MaxTaskNum)
	}
	if m.NextTaskNum != 6 {
		t.Errorf("next = %d", m.NextTaskNum)
	}
}

func TestSlugify(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"Missing auth check on admin endpoint", "missing-auth-check-on-admin-endpoint"},
		{"UPPER CASE", "upper-case"},
		{"  ...  ", "fix"},
		{"SQL injection!?", "sql-injection"},
	}
	for _, c := range cases {
		got := slugify(c.in)
		if got != c.want {
			t.Errorf("slugify(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsHomeConfig(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"/tmp/foo", false},
		{"/home/x/.claudeproject", false},
	}
	for _, guard := range provider.Detect().HomeGuardPaths() {
		cases = append(cases, struct {
			in   string
			want bool
		}{guard, true})
		cases = append(cases, struct {
			in   string
			want bool
		}{filepath.Join(guard, "plugins"), true})
	}
	for _, c := range cases {
		got := IsHomeConfig(c.in)
		if got != c.want {
			t.Errorf("IsHomeConfig(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
