package preflight

import (
	"context"

	"github.com/loldinis/codedungeon/internal/provider"
	"github.com/loldinis/codedungeon/internal/tooladapter"
)

const (
	StatusPass = "PASS"
	StatusWarn = "WARN"
	StatusFail = "FAIL"
)

type Request struct {
	Root       string
	Strict     bool
	Provider   provider.Provider
	Runner     tooladapter.CommandRunner
	FileSystem tooladapter.FileSystem
}

type Report struct {
	OK           bool     `json:"ok"`
	Root         string   `json:"root"`
	Provider     string   `json:"provider,omitempty"`
	Checks       []Check  `json:"checks"`
	NextCommands []string `json:"next_commands,omitempty"`
}

type Check struct {
	ID      string         `json:"id"`
	Status  string         `json:"status"`
	Detail  string         `json:"detail,omitempty"`
	Blocker bool           `json:"blocker,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

type Runner interface {
	Run(ctx context.Context, req Request) (Report, error)
}
