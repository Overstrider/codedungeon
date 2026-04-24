package manifest

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect_Rust(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Cargo.toml"), `
[package]
name = "x"

[dependencies]
poem = "3"
`)
	info, err := Detect(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Lang != "rust" || info.Framework != "poem" {
		t.Errorf("got %+v", info)
	}
	if info.Stack != "Rust + Poem" {
		t.Errorf("stack = %q", info.Stack)
	}
}

func TestDetect_NextJS(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "package.json"), `{"dependencies":{"next":"14","react":"18"}}`)
	info, _ := Detect(dir)
	if info.Lang != "nextjs" {
		t.Errorf("expected nextjs, got %s", info.Lang)
	}
	// Stack should NOT repeat "Nextjs + Next".
	if info.Stack != "Next" {
		t.Errorf("stack = %q; want dedup", info.Stack)
	}
}

func TestDetect_Go_WithChi(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "go.mod"), `module x
require github.com/go-chi/chi/v5 v5.0.0
`)
	info, _ := Detect(dir)
	if info.Lang != "go" || info.Framework != "chi" {
		t.Errorf("got %+v", info)
	}
}

func TestDetect_Unknown(t *testing.T) {
	dir := t.TempDir()
	info, _ := Detect(dir)
	if info.Lang != "unknown" {
		t.Errorf("empty dir should be unknown, got %s", info.Lang)
	}
}

func TestDetect_Kotlin_Compose(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "build.gradle.kts"), `
plugins { id("org.jetbrains.compose") }
`)
	info, _ := Detect(dir)
	if info.Lang != "kotlin" || info.Framework != "compose" {
		t.Errorf("got %+v", info)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
