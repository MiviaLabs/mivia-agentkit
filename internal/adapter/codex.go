// Package adapter defines the Codex CLI adapter.
// Plan: WS-C. PRD: FR-3.1, FR-3.2, FR-7.4.
//
// Codex docs and local CLI verified 2026-07-05:
// https://developers.openai.com/codex/noninteractive documents `codex exec`
// with `--output-last-message`; https://developers.openai.com/codex/cli/reference
// documents pairing `--json` with `--output-last-message`;
// https://developers.openai.com/codex/agent-approvals-security documents
// non-interactive `--sandbox workspace-write` with approval modes; and local
// codex-cli 0.143.0-alpha.21 accepts approval through `-c approval_policy=...`.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// Codex adapts the Codex CLI.
type Codex struct {
	Runner Runner
}

// Name returns the adapter name.
func (Codex) Name() string { return "codex" }

// Role returns the adapter role.
func (Codex) Role() config.AdapterRole { return config.AdapterRoleOrchestrable }

// Detect checks for a Codex CLI binary through the configured runner.
func (c Codex) Detect(ctx context.Context) (Detection, error) {
	res, err := c.runner().Run(ctx, []string{"codex", "--version"}, nil, "")
	return Detection{Name: c.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes Codex non-interactively.
func (c Codex) Run(ctx context.Context, req Request) (Result, error) {
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
	// Capture last-message for campaign typed evidence before sanitize drops payload text.
	// Codex CLI may already have written --output-last-message; materialize scrubs or fills.
	materializeArtifactOut(req.ArtifactOut, res.Stdout)
	return Result{
		ExitCode:     res.ExitCode,
		Stdout:       truncate(sanitizeProviderOutput(res.Stdout)),
		Stderr:       truncate(sanitizeProviderOutput(res.Stderr)),
		ProviderMeta: sanitizedMeta(res.Stdout),
	}, err
}

// Review runs a structured Codex review prompt.
func (c Codex) Review(ctx context.Context, req Request) (Verdict, error) {
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

// ValidateRequest rejects Codex request fields that cannot be passed to Codex CLI.
func (c Codex) ValidateRequest(req Request) error {
	if err := validateCodexConfigValue("approval", req.Approval); err != nil {
		return err
	}
	if err := validateCodexConfigValue("effort", req.Effort); err != nil {
		return err
	}
	if err := req.Validate(); err != nil {
		return err
	}
	if err := validateCodexEffort(req.Effort); err != nil {
		return err
	}
	return validateNoParams(c.Name(), req.Params)
}

func (c Codex) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return OSRunner{}
}

func (c Codex) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"codex", "exec", "--sandbox", "workspace-write", "--config", `approval_policy="` + req.Approval + `"`, "--json"}
	if req.Model != "" {
		args = append(args, "--model", req.Model)
	}
	if req.Effort != "" {
		args = append(args, "--config", `model_reasoning_effort="`+req.Effort+`"`)
	}
	if req.ArtifactOut != "" {
		args = append(args, "--output-last-message", req.ArtifactOut)
	}
	args = append(args, req.Prompt)
	return c.runner().Run(ctx, args, nil, req.Workdir)
}

func validateNoParams(adapterName string, params map[string]string) error {
	if len(params) > 0 {
		return fmt.Errorf("%s unsupported params", adapterName)
	}
	return nil
}

func validateCodexEffort(effort string) error {
	switch effort {
	case "", "minimal", "low", "medium", "high", "xhigh":
		return nil
	default:
		return fmt.Errorf("codex unsupported effort %q", effort)
	}
}

// validateCodexConfigValue rejects values that could break out of --config
// double-quoted strings and inject arbitrary CLI flags.
func validateCodexConfigValue(field, value string) error {
	if strings.Contains(value, `"`) || strings.Contains(value, `\`) {
		return fmt.Errorf("codex %s contains unsafe characters: %q", field, value)
	}
	return nil
}

func sanitizedMeta(stdout []byte) map[string]string {
	meta := map[string]string{}
	for _, payload := range decodeProviderPayloads(stdout) {
		collectMeta(payload, meta)
	}
	return meta
}

func collectMeta(payload map[string]any, meta map[string]string) {
	for _, key := range []string{"model", "model_id", "total_tokens", "usage"} {
		if v, ok := payload[key]; ok {
			meta[key] = strings.TrimSpace(toString(v))
		}
	}
}

func toString(v any) string {
	b, _ := json.Marshal(v)
	return strings.Trim(string(b), `"`)
}

func parseVerdict(b []byte) Verdict {
	var verdict Verdict
	if err := json.Unmarshal(b, &verdict); err != nil || verdict.Severity == "" {
		return Verdict{Pass: false, Severity: "error", Notes: "unparseable review output"}
	}
	return verdict
}

func parseProviderVerdict(b []byte) Verdict {
	if verdict := parseVerdict(b); verdict.Severity != "error" {
		return verdict
	}
	if verdict := parseEmbeddedVerdict(b); verdict.Severity != "error" {
		return verdict
	}
	for _, payload := range decodeProviderPayloads(b) {
		for _, candidate := range rawTextCandidates(payload) {
			if verdict := parseVerdict([]byte(candidate)); verdict.Severity != "error" {
				return verdict
			}
			if verdict := parseEmbeddedVerdict([]byte(candidate)); verdict.Severity != "error" {
				return verdict
			}
		}
	}
	return Verdict{Pass: false, Severity: "error", Notes: "unparseable review output"}
}

func parseEmbeddedVerdict(b []byte) Verdict {
	text := string(b)
	for start := 0; start < len(text); start++ {
		if text[start] != '{' {
			continue
		}
		depth := 0
		inString := false
		escaped := false
		for end := start; end < len(text); end++ {
			ch := text[end]
			if inString {
				if escaped {
					escaped = false
					continue
				}
				if ch == '\\' {
					escaped = true
					continue
				}
				if ch == '"' {
					inString = false
				}
				continue
			}
			switch ch {
			case '"':
				inString = true
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					if verdict := parseVerdict([]byte(text[start : end+1])); verdict.Severity != "error" {
						return verdict
					}
					start = end
				}
			}
		}
	}
	return Verdict{Severity: "error"}
}

func truncate(b []byte) []byte {
	if len(b) <= maxCapturedBytes {
		return b
	}
	return b[:maxCapturedBytes]
}

func sanitizeProviderOutput(b []byte) []byte {
	payloads := decodeProviderPayloads(b)
	if len(payloads) == 0 {
		scrubbed := Scrub(b)
		if !bytes.Equal(scrubbed, b) {
			return scrubbed
		}
		if len(bytes.TrimSpace(b)) == 0 {
			return b
		}
		return []byte("<redacted:provider-output>")
	}
	lines := make([][]byte, 0, len(payloads))
	for _, payload := range payloads {
		dropRawProviderFields(payload)
		line, err := json.Marshal(payload)
		if err != nil {
			return []byte("<redacted:provider-output>")
		}
		lines = append(lines, line)
	}
	return Scrub(bytes.Join(lines, []byte("\n")))
}

func decodeProviderPayloads(b []byte) []map[string]any {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 {
		return nil
	}
	var single map[string]any
	if json.Unmarshal(trimmed, &single) == nil {
		return []map[string]any{single}
	}
	var payloads []map[string]any
	for _, line := range bytes.Split(trimmed, []byte("\n")) {
		var payload map[string]any
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if json.Unmarshal(line, &payload) != nil {
			return nil
		}
		payloads = append(payloads, payload)
	}
	return payloads
}

func dropRawProviderFields(v any) {
	switch typed := v.(type) {
	case map[string]any:
		for _, key := range []string{"prompt", "completion", "result", "text", "content"} {
			delete(typed, key)
		}
		for _, child := range typed {
			dropRawProviderFields(child)
		}
	case []any:
		for _, child := range typed {
			dropRawProviderFields(child)
		}
	}
}

func rawTextCandidates(v any) []string {
	var out []string
	switch typed := v.(type) {
	case map[string]any:
		for _, key := range []string{"response", "result", "text", "content"} {
			if value, ok := typed[key].(string); ok {
				out = append(out, value)
			}
		}
		for _, child := range typed {
			out = append(out, rawTextCandidates(child)...)
		}
	case []any:
		for _, child := range typed {
			out = append(out, rawTextCandidates(child)...)
		}
	}
	return out
}
