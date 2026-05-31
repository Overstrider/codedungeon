package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The embedded settings.json is just `{}`. Merging it over a user's existing
// settings.json must NOT wipe their permissions/hooks (regression for the bug
// where `setup --yes` zeroed .claude/settings.json).
func TestMergeJSONOverExistingPreservesUserSettings(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "settings.json")

	existing := `{
  "permissions": { "allow": ["Bash", "Read", "mcp__graphify"] },
  "hooks": { "SessionStart": [{"hooks":[{"type":"command","command":"x.sh"}]}] }
}`
	if err := os.WriteFile(disk, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	merged, err := mergeJSONOverExisting(disk, []byte("{}"))
	if err != nil {
		t.Fatalf("merge failed: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(merged, &got); err != nil {
		t.Fatalf("merged output not valid JSON: %v\n%s", err, merged)
	}
	if _, ok := got["permissions"]; !ok {
		t.Errorf("permissions wiped by merge; got: %s", merged)
	}
	if _, ok := got["hooks"]; !ok {
		t.Errorf("hooks wiped by merge; got: %s", merged)
	}
}

// New keys from the embedded config are added; nested objects merge; the user's
// existing scalar values win on conflict.
func TestMergeJSONMapsBehaviour(t *testing.T) {
	base := map[string]any{
		"permissions": map[string]any{"allow": []any{"Bash"}},
		"keep":        "user-value",
	}
	incoming := map[string]any{
		"schema":      "https://example/schema.json", // new key -> added
		"keep":        "embedded-value",              // conflict -> user wins
		"permissions": map[string]any{"deny": []any{}}, // nested -> merges
	}
	out := mergeJSONMaps(base, incoming)

	if out["schema"] != "https://example/schema.json" {
		t.Errorf("new key not added: %v", out["schema"])
	}
	if out["keep"] != "user-value" {
		t.Errorf("user scalar should win on conflict, got: %v", out["keep"])
	}
	perms, _ := out["permissions"].(map[string]any)
	if perms == nil || perms["allow"] == nil || perms["deny"] == nil {
		t.Errorf("nested object did not merge, got: %v", out["permissions"])
	}
}

// When no file exists yet, the embedded content is written as-is.
func TestMergeJSONOverExistingWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	disk := filepath.Join(dir, "settings.json")
	out, err := mergeJSONOverExisting(disk, []byte(`{"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `{"a":1}` {
		t.Errorf("expected embedded content when file absent, got: %s", out)
	}
}
