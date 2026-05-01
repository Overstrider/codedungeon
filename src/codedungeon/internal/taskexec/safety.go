package taskexec

import (
	"fmt"
	"regexp"
	"strings"
)

type SafetyPolicy struct {
	AllowedTools []string `json:"allowed_tools"`
}

var destructiveCommandPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[^;&|]*r[^;&|]*f\b`),
	regexp.MustCompile(`(?i)\bgit\s+reset\s+--hard\b`),
	regexp.MustCompile(`(?i)\bgit\s+clean\s+-`),
	regexp.MustCompile(`(?i)\bRemove-Item\b.*\b-Recurse\b.*\b-Force\b`),
	regexp.MustCompile(`(?i)\bdel\s+/[sq]\b`),
}

func (p SafetyPolicy) ValidateShellCommand(command string) error {
	command = strings.TrimSpace(command)
	if command == "" {
		return fmt.Errorf("empty command")
	}
	for _, pattern := range destructiveCommandPatterns {
		if pattern.MatchString(command) {
			return fmt.Errorf("blocked destructive command: %s", command)
		}
	}
	if !p.requiresWhitelist() {
		return nil
	}
	if p.allowed("shell:" + command) {
		return nil
	}
	if strings.HasPrefix(command, "git ") {
		fields := strings.Fields(command)
		if len(fields) > 1 {
			return p.ValidateGit(fields[1])
		}
	}
	return fmt.Errorf("command not whitelisted: %s", command)
}

func (p SafetyPolicy) ValidateGit(subcommand string) error {
	subcommand = strings.TrimSpace(subcommand)
	if subcommand == "" {
		return fmt.Errorf("empty git subcommand")
	}
	if strings.ContainsAny(subcommand, " \t\r\n;&|") {
		subcommand = strings.Fields(subcommand)[0]
	}
	if !p.requiresWhitelist() {
		return nil
	}
	if !p.allowed("git:" + subcommand) {
		return fmt.Errorf("git subcommand not whitelisted: %s", subcommand)
	}
	return nil
}

func (p SafetyPolicy) requiresWhitelist() bool {
	return len(p.AllowedTools) > 0
}

func (p SafetyPolicy) allowed(value string) bool {
	for _, item := range p.AllowedTools {
		if strings.EqualFold(strings.TrimSpace(item), value) {
			return true
		}
	}
	return false
}
