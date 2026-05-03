package taskexec

import (
	"context"
	"strings"
	"testing"

	"github.com/loldinis/codedungeon/internal/tooladapter"
)

type recordingCommandRunner struct {
	calls []tooladapter.Command
}

func (r *recordingCommandRunner) Run(_ context.Context, cmd tooladapter.Command) (tooladapter.CommandResult, error) {
	r.calls = append(r.calls, cmd)
	return tooladapter.CommandResult{}, nil
}

func TestShellGitCommitExcludesRuntimeDatabases(t *testing.T) {
	runner := &recordingCommandRunner{}
	git := ShellGit{Runner: runner}

	if err := git.Commit(context.Background(), "repo", "test"); err != nil {
		t.Fatal(err)
	}

	if len(runner.calls) == 0 {
		t.Fatal("expected git calls")
	}
	call := runner.calls[0]
	got := strings.Join(call.Args, " ")
	want := "add -A -- . :(exclude).codedungeon/*.db :(exclude).codedungeon/*.db-journal :(exclude).codedungeon/logs/**"
	if call.Dir != "repo" || call.Name != "git" || got != want {
		t.Fatalf("unexpected git add call: %+v", call)
	}
}
