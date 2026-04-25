package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/loldinis/codedungeon/internal/provider"
)

type FirstRunDecision struct {
	ShouldSetup bool
	Command     string
}

func DecideFirstRun(providerName string, hasGit, initialized, interactive, confirmed bool) FirstRunDecision {
	if !hasGit || initialized {
		return FirstRunDecision{}
	}
	cmd := providerBinaryName(providerName) + " setup"
	if !interactive {
		return FirstRunDecision{Command: cmd + " --yes"}
	}
	if confirmed {
		return FirstRunDecision{ShouldSetup: true, Command: cmd}
	}
	return FirstRunDecision{Command: cmd}
}

func providerBinaryName(providerName string) string {
	switch providerName {
	case "codex", "codex-cli":
		return "codedungeon-codex"
	case "claude", "claude-code", "claude-ce":
		return "codedungeon-claude"
	default:
		return "codedungeon"
	}
}

func HandleFirstRun(projectRoot string) (bool, error) {
	hasGit := HasGit(projectRoot)
	p := provider.Detect()
	initialized := false
	if _, err := os.Stat(projectDBPath(projectRoot)); err == nil {
		initialized = true
	}

	interactive := isTTY()
	confirmed := false
	if hasGit && !initialized && interactive {
		confirmed = promptYesNo(fmt.Sprintf("Inicializar codedungeon para %s neste projeto?", p.Name()), false)
	}
	decision := DecideFirstRun(p.Name(), hasGit, initialized, interactive, confirmed)
	if decision.ShouldSetup {
		return true, runSetupWithOptions(setupOptions{Target: projectRoot})
	}
	if decision.Command != "" {
		fmt.Fprintf(os.Stderr, "This project is not initialized. Run:\n\n  %s\n\n", decision.Command)
		return true, nil
	}
	return false, nil
}

func projectDBPath(projectRoot string) string {
	return filepath.Join(projectRoot, provider.Detect().DBPath())
}
