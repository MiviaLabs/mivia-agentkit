// Package adapter defines the Claude Code adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
//
// Claude Code docs verified 2026-07-05:
// https://code.claude.com/docs/en/headless documents `claude -p`
// and `--output-format json`; https://docs.anthropic.com/en/docs/claude-code/cli-reference
// documents `--max-turns`; and https://code.claude.com/docs/en/settings
// documents `--permission-mode` overriding settings.
package adapter

import (
	"context"
	"fmt"
	"strings"
)

// Claude adapts the Claude Code CLI.
type Claude struct {
	Runner Runner
}

// Name returns the adapter name.
func (Claude) Name() string { return "claude" }

// Role returns the adapter role.
func (Claude) Role() Role { return RoleOrchestrable }

// Detect checks for a Claude CLI binary through the configured runner.
func (c Claude) Detect(ctx context.Context) (Detection, error) {
	res, err := c.runner().Run(ctx, []string{"claude", "--version"}, nil, "")
	return Detection{Name: c.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes Claude Code in print mode.
func (c Claude) Run(ctx context.Context, req Request) (Result, error) {
	if err := c.ValidateRequest(req); err != nil {
		return Result{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := c.runRaw(runCtx, req)
	return Result{
		ExitCode:     res.ExitCode,
		Stdout:       truncate(sanitizeProviderOutput(res.Stdout)),
		Stderr:       truncate(sanitizeProviderOutput(res.Stderr)),
		ProviderMeta: sanitizedMeta(res.Stdout),
	}, err
}

// Review runs a structured Claude review prompt.
func (c Claude) Review(ctx context.Context, req Request) (Verdict, error) {
	req.Prompt = req.Prompt + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}."
	if err := c.ValidateRequest(req); err != nil {
		return Verdict{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := c.runRaw(runCtx, req)
	if err != nil {
		return Verdict{}, err
	}
	return parseProviderVerdict(res.Stdout), nil
}

// ValidateRequest rejects Claude request fields that cannot be passed to Claude Code.
func (c Claude) ValidateRequest(req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if err := validateClaudeEffort(req.Effort); err != nil {
		return err
	}
	if err := validateClaudeApproval(req.Approval); err != nil {
		return err
	}
	return validateNoParams(c.Name(), req.Params)
}

func (c Claude) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return OSRunner{}
}

func (c Claude) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"claude", "-p", "--output-format", "json", "--permission-mode", req.Approval}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--effort", req.Effort)
	}
	if req.MaxTurns > 0 {
		args = append(args, "--max-turns", toString(req.MaxTurns))
	}
	args = append(args, req.Prompt)
	return c.runner().Run(ctx, args, nil, req.Workdir)
}

func validateClaudeEffort(effort string) error {
	switch effort {
	case "", "low", "medium", "high", "xhigh", "max":
		return nil
	default:
		return fmt.Errorf("claude unsupported effort %q", effort)
	}
}

// validateClaudeApproval rejects unknown Claude Code permission-mode values.
func validateClaudeApproval(approval string) error {
	switch approval {
	case "", "default", "plan", "never", "bypassPermissions":
		return nil
	default:
		return fmt.Errorf("claude unsupported approval %q", approval)
	}
}
