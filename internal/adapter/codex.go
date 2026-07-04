// Package adapter defines the Codex CLI adapter.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
//
// Codex docs verified 2026-07-05:
// https://developers.openai.com/codex/noninteractive documents `codex exec`
// with `--output-last-message`; https://developers.openai.com/codex/cli/reference
// documents pairing `--json` with `--output-last-message`; and
// https://developers.openai.com/codex/agent-approvals-security documents
// non-interactive `--sandbox workspace-write` with approval modes.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
)

// Codex adapts the Codex CLI.
type Codex struct {
	Runner Runner
}

// Name returns the adapter name.
func (Codex) Name() string { return "codex" }

// Role returns the adapter role.
func (Codex) Role() Role { return RoleOrchestrable }

// Detect checks for a Codex CLI binary through the configured runner.
func (c Codex) Detect(ctx context.Context) (Detection, error) {
	res, err := c.runner().Run(ctx, []string{"codex", "--version"}, nil, "")
	return Detection{Name: c.Name(), Version: strings.TrimSpace(string(res.Stdout)), HeadlessCapable: err == nil}, err
}

// Run invokes Codex non-interactively.
func (c Codex) Run(ctx context.Context, req Request) (Result, error) {
	if err := req.Validate(); err != nil {
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

// Review runs a structured Codex review prompt.
func (c Codex) Review(ctx context.Context, req Request) (Verdict, error) {
	req.Prompt = req.Prompt + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}."
	if err := req.Validate(); err != nil {
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

func (c Codex) runner() Runner {
	if c.Runner != nil {
		return c.Runner
	}
	return OSRunner{}
}

func (c Codex) runRaw(ctx context.Context, req Request) (RunResult, error) {
	args := []string{"codex", "exec", "--sandbox", "workspace-write", "--ask-for-approval", req.Approval, "--json"}
	if req.ArtifactOut != "" {
		args = append(args, "--output-last-message", req.ArtifactOut)
	}
	args = append(args, req.Prompt)
	return c.runner().Run(ctx, args, nil, req.Workdir)
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
	for _, payload := range decodeProviderPayloads(b) {
		for _, candidate := range rawTextCandidates(payload) {
			if verdict := parseVerdict([]byte(candidate)); verdict.Severity != "error" {
				return verdict
			}
		}
	}
	return Verdict{Pass: false, Severity: "error", Notes: "unparseable review output"}
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
