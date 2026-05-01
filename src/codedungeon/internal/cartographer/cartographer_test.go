package cartographer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScanRespectsIgnoresAndRendersCompact(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Demo\n")
	writeTestFile(t, filepath.Join(root, "src", "main.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, ".gitignore"), "ignored/\n")
	writeTestFile(t, filepath.Join(root, "ignored", "note.md"), "ignore me\n")
	writeTestFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), "ignore me\n")

	result, err := Scan(root, Options{MaxTokens: 50000})
	if err != nil {
		t.Fatal(err)
	}
	if result.TotalFiles != 3 {
		t.Fatalf("total files = %d, want 3: %+v", result.TotalFiles, result.Files)
	}
	for _, file := range result.Files {
		if strings.Contains(file.Path, "ignored/") || strings.Contains(file.Path, "node_modules/") {
			t.Fatalf("ignored path scanned: %+v", file)
		}
	}
	compact := RenderCompact(result)
	if !strings.Contains(compact, "README.md") || !strings.Contains(compact, "src/main.go") {
		t.Fatalf("compact output missing scanned files:\n%s", compact)
	}
}

func writeTestFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
