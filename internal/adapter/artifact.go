// Package adapter defines headless CLI adapter contracts.
// Plan: WS15. PRD: FR-3.1 campaign typed evidence channel via last-message artifact.
package adapter

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
)

// campaignEvidenceSchema is the only structured last-message body the campaign
// host accepts as commit authority. Kept as a literal here so adapter does not
// import auditcampaign (avoids cycles).
const campaignEvidenceSchema = "mivia-agent-campaign-evidence/v1"

// materializeArtifactOut writes a secret-scrubbed last-message body to ArtifactOut
// using raw provider stdout, before sanitizeProviderOutput drops result/text/content.
//
// Codex may already have written --output-last-message; when the file is non-empty
// we only scrub secrets in place. For Claude/Crush/Zai/Antigravity (no native
// last-message file flag), we extract the assistant last message from raw stdout.
//
// This path is intentionally separate from Result.Stdout: sanitized stdout must
// not carry raw model prose into ordinary run artifacts, but campaign evidence
// is nested inside provider result/text fields and must survive to ArtifactOut.
func materializeArtifactOut(path string, rawStdout []byte) {
	if strings.TrimSpace(path) == "" {
		return
	}
	if st, err := os.Stat(path); err == nil && st.Size() > 0 {
		b, err := os.ReadFile(path)
		if err != nil {
			return
		}
		_ = os.WriteFile(path, Scrub(b), 0o600)
		return
	}
	msg := extractLastMessage(rawStdout)
	if len(bytes.TrimSpace(msg)) == 0 {
		return
	}
	_ = os.WriteFile(path, Scrub(msg), 0o600)
}

// extractLastMessage returns the assistant last-message body from raw provider
// stdout. Prefer a nested campaign-evidence envelope when present; otherwise the
// last non-empty result/text/content string or the trimmed raw payload.
func extractLastMessage(raw []byte) []byte {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	// Prefer an embedded campaign evidence object (schema marker).
	if msg, ok := extractJSONObjectWithMarker(trimmed, campaignEvidenceSchema); ok {
		return msg
	}
	// Provider envelopes: Claude {"result":"..."}, Codex NDJSON, etc.
	for _, payload := range decodeProviderPayloads(trimmed) {
		if msg, ok := extractJSONObjectWithMarker(mustMarshal(payload), campaignEvidenceSchema); ok {
			return msg
		}
		// Nested object under result (not only string).
		if nested, ok := payload["result"].(map[string]any); ok {
			if b, err := json.Marshal(nested); err == nil {
				if bytes.Contains(b, []byte(campaignEvidenceSchema)) {
					return b
				}
			}
		}
		cands := rawTextCandidates(payload)
		for i := len(cands) - 1; i >= 0; i-- {
			c := strings.TrimSpace(cands[i])
			if c == "" {
				continue
			}
			cb := []byte(c)
			if msg, ok := extractJSONObjectWithMarker(cb, campaignEvidenceSchema); ok {
				return msg
			}
			// Last non-empty text candidate (may itself be bare evidence JSON).
			if i == len(cands)-1 || bytes.Contains(cb, []byte(campaignEvidenceSchema)) {
				return cb
			}
		}
		if len(cands) > 0 {
			return []byte(cands[len(cands)-1])
		}
	}
	// Bare last-message JSON or plain text on stdout.
	return trimmed
}

func extractJSONObjectWithMarker(raw []byte, marker string) ([]byte, bool) {
	// Prefer the innermost/valid campaign-evidence object, not an outer provider
	// wrapper (e.g. {"role":"assistant","content":"...schema..."}).
	var best []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] != '{' {
			continue
		}
		rest := raw[i:]
		if !bytes.Contains(rest, []byte(marker)) {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(rest))
		var msg json.RawMessage
		if err := dec.Decode(&msg); err != nil {
			continue
		}
		// Prefer objects that look like the campaign evidence envelope.
		var probe map[string]any
		if err := json.Unmarshal(msg, &probe); err != nil {
			continue
		}
		if sch, _ := probe["schema"].(string); sch == marker {
			// Exact evidence envelope wins immediately.
			return msg, true
		}
		// Keep last decoded object containing the marker as weak fallback.
		best = msg
		// Also search string fields for nested evidence JSON.
		for _, v := range probe {
			s, ok := v.(string)
			if !ok || !strings.Contains(s, marker) {
				continue
			}
			if nested, ok := extractJSONObjectWithMarker([]byte(s), marker); ok {
				return nested, true
			}
		}
	}
	if len(best) > 0 {
		return best, true
	}
	return nil, false
}

func mustMarshal(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
