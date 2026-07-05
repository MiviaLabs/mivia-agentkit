// Package adapter defines the Google Antigravity CLI adapter.
// Plan: WS6. PRD: FR-3.1, FR-3.4.
//
// Google docs verified 2026-07-05:
// https://developers.googleblog.com/an-important-update-transitioning-gemini-cli-to-antigravity-cli/
// documents the consumer transition from Gemini CLI to Antigravity CLI and
// that Gemini CLI stopped serving most individual users on 2026-06-18.
// Antigravity CLI uses the `agy` binary; public docs and examples show `agy -p`
// for a one-shot prompt. Gemini CLI is no longer targeted by this adapter.
package adapter

import (
	"context"
	"fmt"
	"strings"
)

// Antigravity adapts Google's current Antigravity CLI.
type Antigravity struct {
	Runner Runner
}

// Name returns the adapter name.
func (Antigravity) Name() string { return "antigravity" }

// Role returns the adapter role.
func (Antigravity) Role() Role { return RoleOrchestrable }

// Detect checks for an Antigravity CLI binary through the configured runner.
func (g Antigravity) Detect(ctx context.Context) (Detection, error) {
	res, err := g.runner().Run(ctx, []string{"agy", "--version"}, nil, "")
	return Detection{Name: g.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes Antigravity CLI with a one-shot prompt.
func (g Antigravity) Run(ctx context.Context, req Request) (Result, error) {
	if err := g.ValidateRequest(req); err != nil {
		return Result{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := g.runRaw(runCtx, req)
	return Result{
		ExitCode:     res.ExitCode,
		Stdout:       truncate(sanitizeProviderOutput(res.Stdout)),
		Stderr:       truncate(sanitizeProviderOutput(res.Stderr)),
		ProviderMeta: sanitizedMeta(res.Stdout),
	}, err
}

// Review runs a structured Antigravity review prompt.
func (g Antigravity) Review(ctx context.Context, req Request) (Verdict, error) {
	req.Prompt = req.Prompt + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}."
	if err := g.ValidateRequest(req); err != nil {
		return Verdict{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := g.runRaw(runCtx, req)
	if err != nil {
		return Verdict{}, err
	}
	return parseProviderVerdict(res.Stdout), nil
}

// ValidateRequest rejects Antigravity request fields without a documented CLI mapping.
func (g Antigravity) ValidateRequest(req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if req.Model != "" {
		return fmt.Errorf("%s unsupported model %q", g.Name(), req.Model)
	}
	if req.Effort != "" {
		return fmt.Errorf("%s unsupported effort %q", g.Name(), req.Effort)
	}
	return validateNoParams(g.Name(), req.Params)
}

func (g Antigravity) runner() Runner {
	if g.Runner != nil {
		return g.Runner
	}
	return OSRunner{}
}

func (g Antigravity) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"agy", "-p", req.Prompt}
	return g.runner().Run(ctx, args, nil, req.Workdir)
}
