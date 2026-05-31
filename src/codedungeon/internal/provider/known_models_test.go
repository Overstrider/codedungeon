package provider

import "testing"

func TestKnownModelsCoversDefaultsAndAlternatives(t *testing.T) {
	known := KnownModels(&Claude{})
	// Current default + the model path-sale pins must be recognised (no false typo warning).
	for _, m := range []string{"claude-opus-4-7", "claude-sonnet-4-6", "claude-opus-4-8"} {
		if !known[m] {
			t.Errorf("expected %q in KnownModels(claude)", m)
		}
	}
	if known["claude-opus-4-8x"] {
		t.Error("a typo'd model should not be in KnownModels")
	}
	if len(known) == 0 {
		t.Error("KnownModels returned empty set")
	}
}
