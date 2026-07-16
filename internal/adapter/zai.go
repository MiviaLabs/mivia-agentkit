// Package adapter defines the ZAI CLI adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
//
// ZAI CLI verified 2026-07-16 against local @guizmo-ai/zai-cli v0.3.5:
// `zai --help` documents `zai -p <prompt>` headless mode, `-m <model>`,
// `-d <directory>`, `-k <api-key>` (or ZAI_API_KEY env), `--no-color`, and
// `--max-tool-rounds <rounds>`. Live probes confirmed `glm-5.2` and
// `glm-5-turbo` are accepted by the Z.ai api/coding/paas/v4 endpoint; invalid
// models yield a non-fatal stderr error but exit code 0. Headless output is
// JSON-lines on stdout (one object per user/assistant/tool message) with empty
// stderr on success. ZAI CLI has no approval/permission or reasoning-effort
// flag, so those request fields are validated as unsupported.
package adapter

import (
	"context"
	"fmt"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// zaiDefaultModel is used when req.Model is empty. GLM-5.2 is the Z.ai
// flagship verified against the coding endpoint on 2026-07-16.
const zaiDefaultModel = "glm-5.2"

// Zai adapts the @guizmo-ai/zai-cli (binary: zai) for Z.ai GLM models.
type Zai struct {
	Runner Runner
}

// Name returns the adapter name.
func (Zai) Name() string { return "zai" }

// Role returns the adapter role.
func (Zai) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }

// Detect checks for a zai CLI binary through the configured runner.
func (z Zai) Detect(ctx context.Context) (Detection, error) {
	res, err := z.runner().Run(ctx, []string{"zai", "--version"}, nil, "")
	return Detection{Name: z.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes zai in headless mode.
func (z Zai) Run(ctx context.Context, req Request) (Result, error) {
	if err := z.ValidateRequest(req); err != nil {
		return Result{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := z.runRaw(runCtx, req)
	return Result{
		ExitCode:     res.ExitCode,
		Stdout:       truncate(sanitizeProviderOutput(res.Stdout)),
		Stderr:       truncate(sanitizeProviderOutput(res.Stderr)),
		ProviderMeta: sanitizedMeta(res.Stdout),
	}, err
}

// Review runs a structured zai review prompt.
func (z Zai) Review(ctx context.Context, req Request) (Verdict, error) {
	req.Prompt = req.Prompt + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}."
	if err := z.ValidateRequest(req); err != nil {
		return Verdict{}, err
	}
	runCtx := ctx
	var cancel context.CancelFunc
	if req.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}
	res, err := z.runRaw(runCtx, req)
	if err != nil {
		return Verdict{}, err
	}
	return parseProviderVerdict(res.Stdout), nil
}

// ValidateRequest rejects zai request fields that cannot be passed to the zai CLI.
// ZAI CLI headless mode has no permission/approval surface and no reasoning-effort
// flag; the orchestrator-supplied "never" sentinel is accepted as the canonical
// no-approval value. The only supported override param is "model" (forwarded as -m).
func (z Zai) ValidateRequest(req Request) error {
	if err := req.Validate(); err != nil {
		return err
	}
	if err := validateZaiApproval(req.Approval); err != nil {
		return err
	}
	if req.Effort != "" {
		return fmt.Errorf("zai unsupported effort %q", req.Effort)
	}
	return validateZaiParams(req.Params)
}

func (z Zai) runner() Runner {
	if z.Runner != nil {
		return z.Runner
	}
	return OSRunner{}
}

func (z Zai) runRaw(ctx context.Context, req Request) (RunResult, error) {
	model := req.Model
	if model == "" {
		if m, ok := req.Params["model"]; ok && m != "" {
			model = m
		}
	}
	if model == "" {
		model = zaiDefaultModel
	}
	args := []string{"zai", "-m", model, "-p", req.Prompt, "--no-color"}
	if req.Workdir != "" {
		args = append(args, "-d", req.Workdir)
	}
	if req.MaxTurns > 0 {
		args = append(args, "--max-tool-rounds", toString(req.MaxTurns))
	}
	// zai has no native artifact-file flag; headless output is emitted on stdout
	// and surfaced via Result.Stdout. req.ArtifactOut, if any, is honored by the
	// caller from the returned result rather than by the CLI.
	return z.runner().Run(ctx, args, nil, req.Workdir)
}

// validateZaiApproval accepts only the orchestrator default "never" sentinel:
// zai headless -p mode never prompts for permission, so there is no other
// valid approval value to honor. An empty approval is already rejected by
// Request.Validate as a required field.
func validateZaiApproval(approval string) error {
	if approval == "never" {
		return nil
	}
	return fmt.Errorf("zai unsupported approval %q", approval)
}

// validateZaiParams allows only the optional "model" override param.
func validateZaiParams(params map[string]string) error {
	for k := range params {
		if k != "model" {
			return fmt.Errorf("zai unsupported params")
		}
	}
	return nil
}
