package cmd

import (
	"path/filepath"
	"strings"

	"github.com/loldinis/codedungeon/internal/db"
	"github.com/loldinis/codedungeon/internal/provider"
)

func providerChildModel(root, mode string, p provider.Provider) string {
	if p == nil || p.Name() != "claude" {
		return ""
	}
	defaultModel := p.DefaultModels().Reasoning
	return configuredModelAtRoot(root, providerChildModelTier(mode), defaultModel)
}

func providerChildModelTier(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "full", "lite", "oneshot", "rules":
		return "reasoning"
	default:
		return "reasoning"
	}
}

func configuredModelAtRoot(root, tier, fallback string) string {
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	tier = strings.TrimSpace(tier)
	if tier == "" {
		return strings.TrimSpace(fallback)
	}
	store, err := db.Open(filepath.Join(root, provider.Detect().DBPath()))
	if err != nil {
		return strings.TrimSpace(fallback)
	}
	defer store.Close()
	lock, _ := store.GetMeta("model_lock")
	model, err := store.GetMeta("model_" + tier)
	if strings.TrimSpace(lock) != "" {
		return strings.TrimSpace(lock)
	}
	if err != nil || strings.TrimSpace(model) == "" {
		return strings.TrimSpace(fallback)
	}
	return strings.TrimSpace(model)
}
