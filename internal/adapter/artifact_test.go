// Package adapter tests last-message artifact materialization.
// Plan: WS15. PRD: campaign typed evidence survives provider sanitization.
package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func campaignEvidenceBody(disposition, fingerprint string) []byte {
	extra := ""
	if disposition == "confirmed" || disposition == "fixed" {
		extra = `,"changed_path_ids":["p1"],"verifier_ref":"true"`
	}
	fp := ""
	if fingerprint != "" {
		fp = `,"finding_fingerprint":"` + fingerprint + `"`
	}
	disp := ""
	if disposition != "" {
		disp = `,"disposition":"` + disposition + `"`
	}
	return []byte(`{"schema":"mivia-agent-campaign-evidence/v1","campaign_run":"r","cycle":1,"baseline_head":"h"` + disp + fp + extra + `}`)
}

func TestExtractLastMessageFromClaudeResultEnvelope(t *testing.T) {
	inner := campaignEvidenceBody("confirmed", "fp-claude")
	// Claude --output-format json nests assistant text in result as a string.
	enc, err := json.Marshal(string(inner))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"type":"result","subtype":"success","result":` + string(enc) + `}`)
	got := extractLastMessage(raw)
	if !bytes.Contains(got, []byte(`"schema":"mivia-agent-campaign-evidence/v1"`)) {
		t.Fatalf("extractLastMessage = %s, want campaign evidence schema", got)
	}
	if !bytes.Contains(got, []byte(`"fp-claude"`)) {
		t.Fatalf("extractLastMessage = %s, want fingerprint", got)
	}
}

func TestExtractLastMessageFromNestedResultObject(t *testing.T) {
	inner := campaignEvidenceBody("candidate", "fp-obj")
	var obj map[string]any
	if err := json.Unmarshal(inner, &obj); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"type": "result", "result": obj})
	if err != nil {
		t.Fatal(err)
	}
	got := extractLastMessage(raw)
	if !bytes.Contains(got, []byte("fp-obj")) {
		t.Fatalf("got %s", got)
	}
}

func TestMaterializeArtifactOutWritesBeforeSanitizeWouldDrop(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	inner := campaignEvidenceBody("confirmed", "fp-mat")
	enc, _ := json.Marshal(string(inner))
	raw := []byte(`{"type":"result","result":` + string(enc) + `}`)

	// Prove sanitization would destroy the evidence if we only had stdout.
	sanitized := sanitizeProviderOutput(raw)
	if bytes.Contains(sanitized, []byte("mivia-agent-campaign-evidence/v1")) {
		t.Fatalf("sanitize unexpectedly kept evidence: %s", sanitized)
	}

	materializeArtifactOut(path, raw)
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(got, []byte("mivia-agent-campaign-evidence/v1")) || !bytes.Contains(got, []byte("fp-mat")) {
		t.Fatalf("artifact = %s, want campaign evidence", got)
	}
}

func TestClaudeRunMaterializesArtifactOutFromFakeRunner(t *testing.T) {
	inner := campaignEvidenceBody("confirmed", "fp-run")
	enc, _ := json.Marshal(string(inner))
	raw := []byte(`{"type":"result","result":` + string(enc) + `}`)
	r := &FakeRunner{Scripts: map[string]FakeResponse{
		"claude": {Result: RunResult{ExitCode: 0, Stdout: raw}},
	}}
	path := filepath.Join(t.TempDir(), "last.json")
	res, err := (Claude{Runner: r}).Run(context.Background(), Request{
		Prompt: "emit evidence", Approval: "never", ArtifactOut: path,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Sanitized stdout must not be the only channel — ArtifactOut carries evidence.
	if bytes.Contains(res.Stdout, []byte("mivia-agent-campaign-evidence/v1")) {
		// optional: may or may not depending on sanitize; primary assert is file
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ArtifactOut missing: %v", err)
	}
	if !bytes.Contains(got, []byte("fp-run")) {
		t.Fatalf("ArtifactOut = %s", got)
	}
	// Stdout after Run must have dropped raw result text (fail-closed for ordinary runs).
	if bytes.Contains(res.Stdout, []byte("fp-run")) {
		t.Fatalf("sanitized Stdout still contains evidence body: %s", res.Stdout)
	}
}

func TestCodexRunMaterializesArtifactOutWhenCLIDidNotWrite(t *testing.T) {
	// FakeRunner does not implement --output-last-message; adapter must still materialize.
	inner := campaignEvidenceBody("candidate", "fp-codex")
	// Codex often emits NDJSON; include a final message-like object with text.
	line, _ := json.Marshal(map[string]any{"type": "item.completed", "text": string(inner)})
	r := &FakeRunner{Scripts: map[string]FakeResponse{
		"codex": {Result: RunResult{ExitCode: 0, Stdout: line}},
	}}
	path := filepath.Join(t.TempDir(), "codex-last.json")
	_, err := (Codex{Runner: r}).Run(context.Background(), Request{
		Prompt: "audit", Approval: "never", ArtifactOut: path,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ArtifactOut: %v", err)
	}
	if !bytes.Contains(got, []byte("fp-codex")) {
		t.Fatalf("ArtifactOut = %s", got)
	}
	// Ensure --output-last-message flag still present for real CLI.
	args := strings.Join(r.Calls[0].Args, " ")
	if !strings.Contains(args, "--output-last-message") || !strings.Contains(args, path) {
		t.Fatalf("args = %q, want --output-last-message %s", args, path)
	}
}
