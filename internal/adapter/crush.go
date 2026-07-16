// Package adapter defines the Crush adapter.
// Plan: WS6/WS9/WS14. PRD: FR-3.1, FR-3.2, FR-3.4, FR-7.4.
//
// Crush CLI verified 2026-07-05 against local v0.79.1:
// `crush run --help` documents `crush run [prompt...]`, non-interactive mode,
// stdin prompt input, `--cwd`, `--model`, `--small-model`, `--quiet`,
// `--session`, and `--continue`.
package adapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// ErrNotHeadlessCapable indicates an adapter cannot run non-interactively.
var ErrNotHeadlessCapable = errors.New("adapter is not headless capable")

// Crush adapts the Crush CLI.
type Crush struct {
	Runner Runner
}

// Name returns the adapter name.
func (Crush) Name() string { return "crush" }

// Role returns the adapter role.
func (Crush) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }

// Detect checks for a Crush CLI binary and verifies run help before approving headless use.
func (c Crush) Detect(ctx context.Context) (Detection, error) {
	version, err := c.runner().Run(ctx, []string{"crush", "--version"}, nil, "")
	d := Detection{Name: c.Name(), Version: strings.TrimSpace(string(version.Stdout))}
	if err != nil {
		return d, err
	}
	help, err := c.runner().Run(ctx, []string{"crush", "run", "--help"}, nil, "")
	if err != nil {
		return d, nil
	}
	d.HeadlessCapable = confirmsCrushRunSupport(append(help.Stdout, help.Stderr...))
	return d, nil
}

// Run invokes Crush non-interactively.
func (c Crush) Run(ctx context.Context, req Request) (Result, error) {
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
		ExitCode: res.ExitCode,
		Stdout:   truncate(Scrub(res.Stdout)),
		Stderr:   truncate(Scrub(res.Stderr)),
	}, err
}

// Review runs a structured Crush review prompt.
func (c Crush) Review(ctx context.Context, req Request) (Verdict, error) {
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

// ValidateRequest rejects Crush request fields that cannot be passed to Crush CLI.
func (c Crush) ValidateRequest(req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if req.Approval != "never" {
		return fmt.Errorf("crush unsupported approval %q", req.Approval)
	}
	if req.Effort != "" {
		return fmt.Errorf("crush unsupported effort %q", req.Effort)
	}
	return validateNoParams(c.Name(), req.Params)
}

func (c Crush) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return OSRunner{}
}

func (c Crush) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"crush", "run", "--quiet", "--cwd", req.Workdir}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	r := c.runner()
	inputRunner, ok := r.(InputRunner)
	if !ok {
		return RunResult{}, errors.New("crush runner does not support stdin")
	}
	return inputRunner.RunWithInput(ctx, args, nil, "", []byte(req.Prompt))
}

func confirmsCrushRunSupport(help []byte) bool {
	text := strings.ToLower(string(help))
	return strings.Contains(text, "crush run") &&
		strings.Contains(text, "non-interactive") &&
		strings.Contains(text, "stdin")
}
