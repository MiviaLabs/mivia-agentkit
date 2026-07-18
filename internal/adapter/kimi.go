// Package adapter defines the Kimi Code CLI adapter.
// Plan: WS15. PRD: FR-3.1, FR-3.2 campaign orchestrable dual-CLI dogfood.
//
// Kimi Code CLI (binary: kimi) verified 2026-07-18:
// `kimi -p <prompt>` headless mode prints the assistant reply and exits.
// `--yolo` cannot combine with `-p`. Optional `-m <model>`. No native
// output-schema or last-message file flag.
package adapter

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// Kimi adapts the Kimi Code CLI for headless campaign phases.
type Kimi struct {
	Runner Runner
}

// Name returns the adapter name.
func (Kimi) Name() string { return "kimi" }

// Role returns the adapter role.
func (Kimi) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }

// Detect checks for a kimi CLI binary through the configured runner.
func (k Kimi) Detect(ctx context.Context) (Detection, error) {
	res, err := k.runner().Run(ctx, []string{"kimi", "--version"}, nil, "")
	return Detection{Name: k.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes kimi in headless prompt mode.
func (k Kimi) Run(ctx context.Context, req Request) (Result, error) {
	if err := k.ValidateRequest(req); err != nil {
		return Result{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := k.runRaw(runCtx, req)
	if err != nil {
		return Result{}, err
	}
	if fail, why := kimiProviderFailure(res); fail {
		return Result{
			ExitCode: 1,
			Stdout:   truncate(sanitizeProviderOutput(res.Stdout)),
			Stderr:   truncate(sanitizeProviderOutput(res.Stderr)),
		}, fmt.Errorf("kimi provider failure: %s", why)
	}
	materializeArtifactOut(req.ArtifactOut, res.Stdout)
	return Result{
		ExitCode: res.ExitCode,
		Stdout:   truncate(sanitizeProviderOutput(res.Stdout)),
		Stderr:   truncate(sanitizeProviderOutput(res.Stderr)),
	}, nil
}

// Review runs a structured kimi review prompt.
func (k Kimi) Review(ctx context.Context, req Request) (Verdict, error) {
	req.Prompt = req.Prompt + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}."
	if err := k.ValidateRequest(req); err != nil {
		return Verdict{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := k.runRaw(runCtx, req)
	if err != nil {
		return Verdict{}, err
	}
	return parseProviderVerdict(res.Stdout), nil
}

// ValidateRequest rejects kimi request fields that cannot be passed to the CLI.
// Headless -p mode accepts only the orchestrator "never" approval sentinel.
func (k Kimi) ValidateRequest(req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if req.Approval != "never" {
		return fmt.Errorf("kimi unsupported approval %q", req.Approval)
	}
	if req.Effort != "" {
		return fmt.Errorf("kimi unsupported effort %q", req.Effort)
	}
	return validateNoParams(k.Name(), req.Params)
}

func (k Kimi) runner() Runner {
	if k.Runner != nil {
		return k.Runner
	}
	return OSRunner{}
}

func (k Kimi) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"kimi", "-p", req.Prompt}
	if req.Model != "" {
		args = append(args, "-m", req.Model)
	}
	// Kimi has no -d workdir flag; run with process workdir when provided.
	return k.runner().Run(ctx, args, nil, req.Workdir)
}

func kimiProviderFailure(res RunResult) (bool, string) {
	// Only treat short standalone failure replies as fatal. Long agent
	// transcripts often quote "401" / "authentication failed" from repo docs.
	if isShortProviderFailure(res.Stderr) {
		return true, "authentication or API error in stderr"
	}
	if isShortProviderFailure(res.Stdout) {
		return true, "authentication or API error in provider output"
	}
	// Last-message body only (not full tool log).
	if msg := extractLastMessage(res.Stdout); isShortProviderFailure(msg) {
		return true, "authentication or API error in last message"
	}
	return false, ""
}

// isShortProviderFailure is true when b is a compact provider auth/API error,
// not a long tool transcript that merely mentions those words.
func isShortProviderFailure(b []byte) bool {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || len(trimmed) > 800 {
		return false
	}
	return isProviderFailureMessage(trimmed)
}
