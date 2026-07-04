// Package adapter defines the Crush adapter.
// Plan: WS6. PRD: FR-3.1, FR-3.4.
//
// Crush docs verified 2026-07-05:
// https://github.com/charmbracelet/crush documents interactive configuration
// but no stable non-TUI run command; https://github.com/charmbracelet/crush/issues/1862
// records a request for fully headless automation semantics. Until a documented
// headless mode exists, Crush is guidance-only and excluded from orchestrated runs.
package adapter

import (
	"context"
	"errors"
	"strings"
)

// ErrNotHeadlessCapable indicates an adapter cannot run non-interactively.
var ErrNotHeadlessCapable = errors.New("adapter is not headless capable")

// Crush adapts the Crush CLI when present, but only as guidance.
type Crush struct {
	Runner Runner
}

// Name returns the adapter name.
func (Crush) Name() string { return "crush" }

// Role returns the adapter role.
func (Crush) Role() Role { return RoleGuidance }

// Detect checks for a Crush CLI binary and reports it as not headless-capable.
func (c Crush) Detect(ctx context.Context) (Detection, error) {
	res, err := c.runner().Run(ctx, []string{"crush", "--version"}, nil, "")
	return Detection{Name: c.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: false}, err
}

// Run rejects Crush orchestration until Crush has a documented headless mode.
func (c Crush) Run(context.Context, Request) (Result, error) {
	return Result{}, ErrNotHeadlessCapable
}

// Review rejects Crush orchestration until Crush has a documented headless mode.
func (c Crush) Review(context.Context, Request) (Verdict, error) {
	return Verdict{}, ErrNotHeadlessCapable
}

func (c Crush) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return OSRunner{}
}
