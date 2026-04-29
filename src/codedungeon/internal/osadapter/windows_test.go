//go:build windows

package osadapter

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsFindToolFindsPortableGHOutsidePath(t *testing.T) {
	root := t.TempDir()
	portableBin := filepath.Join(root, "gh-portable", "bin")
	if err := os.MkdirAll(portableBin, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(portableBin, "gh.exe")
	if err := os.WriteFile(want, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	oldHints := extraHints
	extraHints = nil
	t.Cleanup(func() { extraHints = oldHints })
	t.Setenv("PATH", "")
	t.Setenv("TEMP", root)
	t.Setenv("TMP", root)
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "local"))
	t.Setenv("ProgramFiles", filepath.Join(root, "program-files"))
	t.Setenv("ProgramFiles(x86)", filepath.Join(root, "program-files-x86"))
	t.Setenv("USERPROFILE", filepath.Join(root, "user"))
	t.Setenv("ProgramData", filepath.Join(root, "program-data"))

	got, err := (windowsAdapter{}).FindTool("gh")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindTool(\"gh\") = %q, want %q", got, want)
	}
}

func TestWindowsFindToolHonorsExplicitOverride(t *testing.T) {
	root := t.TempDir()
	want := filepath.Join(root, "custom-gh.exe")
	if err := os.WriteFile(want, []byte(""), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	t.Setenv("CODEDUNGEON_GH_PATH", want)

	got, err := (windowsAdapter{}).FindTool("gh")
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("FindTool(\"gh\") = %q, want %q", got, want)
	}
}
