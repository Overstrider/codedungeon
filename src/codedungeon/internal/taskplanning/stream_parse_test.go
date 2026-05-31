package taskplanning

import (
	"encoding/json"
	"testing"
)

// The `claude --output-format stream-json` output is NDJSON: a system init line,
// one or more assistant message lines, then a result line. We must reconstruct the
// assistant text and isolate the agent's JSON object (which itself contains braces
// and quoted strings).
func TestExtractAgentJSONFromStreamAssistant(t *testing.T) {
	stream := `{"type":"system","subtype":"init","session_id":"abc"}
{"type":"assistant","message":{"content":[{"type":"text","text":"Here is the plan:\n{\"role\":\"qa_planner\",\"agent_name\":\"owl\",\"notes\":\"check {edge} cases\",\"tasks\":[1,2]}"}]}}
{"type":"result","subtype":"success","result":"done"}`

	got, err := extractAgentJSONFromStream(stream)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("extracted not valid JSON: %v\n%s", err, got)
	}
	if obj["role"] != "qa_planner" || obj["agent_name"] != "owl" {
		t.Errorf("wrong object extracted: %s", got)
	}
}

// Some versions stream text via content_block_delta events.
func TestExtractAgentJSONFromStreamDeltas(t *testing.T) {
	stream := `{"type":"content_block_delta","delta":{"type":"text_delta","text":"{\"role\":\"risk"}}
{"type":"content_block_delta","delta":{"type":"text_delta","text":"_planner\",\"ok\":true}"}}`
	got, err := extractAgentJSONFromStream(stream)
	if err != nil {
		t.Fatalf("extract failed: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, got)
	}
	if obj["role"] != "risk_planner" {
		t.Errorf("delta reconstruction failed: %s", got)
	}
}

// Empty / non-JSON stream must error (so the runner fails loudly, not silently).
func TestExtractAgentJSONFromStreamEmpty(t *testing.T) {
	if _, err := extractAgentJSONFromStream(""); err == nil {
		t.Error("expected error for empty stream")
	}
	if _, err := extractAgentJSONFromStream("no json here\njust logs"); err == nil {
		t.Error("expected error for stream without JSON")
	}
}

// Braces inside string literals must not break the balance counter.
func TestFirstBalancedJSONObjectRespectsStrings(t *testing.T) {
	s := `prefix noise {"a":"}{","b":{"c":1}} trailing`
	got := firstBalancedJSONObject(s)
	if !json.Valid([]byte(got)) {
		t.Fatalf("not valid: %q", got)
	}
	var obj map[string]any
	_ = json.Unmarshal([]byte(got), &obj)
	if obj["a"] != "}{" {
		t.Errorf("string-aware balance failed: %q", got)
	}
}
