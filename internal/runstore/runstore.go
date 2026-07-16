// Package runstore persists bounded workflow run artifacts.
// Plan: WS10. PRD: FR-4.5, FR-7.4.
package runstore

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

// RunID identifies one orchestrator run.
type RunID string

// Store writes run data under <repo>/.ai/runs.
type Store struct{ Root string }

// TraceEvent is one JSONL trace event.
type TraceEvent struct {
	TS        string         `json:"ts"`
	Kind      string         `json:"kind"`
	Step      string         `json:"step"`
	Iteration int            `json:"iteration"`
	Payload   map[string]any `json:"payload"`
}

// New returns a store rooted at repo/.ai/runs.
func New(repo string) Store {
	if repo == "" {
		repo = "."
	}
	return Store{Root: filepath.Join(repo, ".ai", "runs")}
}

// NewRun creates and returns a new run ID after ensuring the run directory exists.
func (s Store) NewRun() (RunID, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate run id: %w", err)
	}
	id := RunID(time.Now().UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(b[:]))
	if err := os.MkdirAll(s.Dir(id), 0o755); err != nil {
		return "", fmt.Errorf("create run dir: %w", err)
	}
	return id, nil
}

// Dir returns the absolute directory for id.
func (s Store) Dir(id RunID) string {
	abs, err := filepath.Abs(filepath.Join(s.root(), string(id)))
	if err != nil {
		return filepath.Join(s.root(), string(id))
	}
	return abs
}

// WriteArtifact writes one step artifact and returns its absolute path.
func (s Store) WriteArtifact(id RunID, step string, iteration int, name string, b []byte) (string, error) {
	rel, abs, err := s.checkedArtifactPath(id, step, iteration, name)
	if err != nil {
		return "", err
	}
	if err := pathpolicy.NewDefault().Check(s.repoRoot(), rel); err != nil {
		return "", fmt.Errorf("run artifact path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", fmt.Errorf("create artifact dir: %w", err)
	}
	if err := os.WriteFile(abs, b, 0o644); err != nil {
		return "", fmt.Errorf("write artifact: %w", err)
	}
	return abs, nil
}

// ReadArtifact reads a step artifact.
func (s Store) ReadArtifact(id RunID, step string, iteration int, name string) ([]byte, error) {
	_, abs, err := s.checkedArtifactPath(id, step, iteration, name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, fmt.Errorf("read artifact: %w", err)
	}
	return b, nil
}

// AppendTrace appends one deterministic JSON trace line.
func (s Store) AppendTrace(id RunID, event TraceEvent) error {
	runDir, err := s.checkedRunDir(id)
	if err != nil {
		return err
	}
	if event.TS == "" {
		event.TS = time.Now().UTC().Format(time.RFC3339)
	}
	if event.Payload == nil {
		event.Payload = map[string]any{}
	}
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	line, err := marshalTrace(event)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(runDir, "trace.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open trace: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write trace: %w", err)
	}
	return nil
}

func (s Store) checkedRunDir(id RunID) (string, error) {
	if id == "" {
		return "", fmt.Errorf("run id is required")
	}
	if filepath.IsAbs(string(id)) {
		return "", fmt.Errorf("run path must be relative")
	}
	if hasTraversal(string(id)) {
		return "", fmt.Errorf("run path traverses outside runs")
	}
	runs, err := filepath.Abs(filepath.Join(s.repoRoot(), ".ai", "runs"))
	if err != nil {
		return "", err
	}
	checked, err := filepath.Abs(filepath.Join(runs, string(id)))
	if err != nil {
		return "", err
	}
	if checked != runs && !strings.HasPrefix(checked, runs+string(filepath.Separator)) {
		return "", fmt.Errorf("run path escapes runs")
	}
	return checked, nil
}

func (s Store) checkedArtifactPath(id RunID, step string, iteration int, name string) (string, string, error) {
	if id == "" || step == "" || iteration <= 0 || name == "" {
		return "", "", fmt.Errorf("run id, step, iteration, and name are required")
	}
	for _, raw := range []string{string(id), step, name} {
		if hasTraversal(raw) {
			return "", "", fmt.Errorf("artifact path traverses outside runs")
		}
	}
	rel := filepath.Join(".ai", "runs", string(id), step, fmt.Sprintf("iter-%03d", iteration), name)
	abs := filepath.Join(s.repoRoot(), rel)
	runs, err := filepath.Abs(filepath.Join(s.repoRoot(), ".ai", "runs"))
	if err != nil {
		return "", "", err
	}
	checked, err := filepath.Abs(abs)
	if err != nil {
		return "", "", err
	}
	if checked != runs && !strings.HasPrefix(checked, runs+string(filepath.Separator)) {
		return "", "", fmt.Errorf("artifact path escapes runs")
	}
	return rel, checked, nil
}

func hasTraversal(raw string) bool {
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == '/' || r == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

func (s Store) root() string {
	if s.Root == "" {
		return filepath.Join(".", ".ai", "runs")
	}
	return s.Root
}

func (s Store) repoRoot() string { return filepath.Dir(filepath.Dir(s.root())) }

func marshalTrace(event TraceEvent) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`{"iteration":`)
	b.WriteString(fmt.Sprint(event.Iteration))
	b.WriteString(`,"kind":`)
	if err := writeJSON(&b, event.Kind); err != nil {
		return nil, err
	}
	b.WriteString(`,"payload":{`)
	keys := make([]string, 0, len(event.Payload))
	for k := range event.Payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for i, k := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		if err := writeJSON(&b, k); err != nil {
			return nil, err
		}
		b.WriteByte(':')
		if err := writeJSON(&b, event.Payload[k]); err != nil {
			return nil, fmt.Errorf("marshal trace payload %q: %w", k, err)
		}
	}
	b.WriteString(`},"step":`)
	if err := writeJSON(&b, event.Step); err != nil {
		return nil, err
	}
	b.WriteString(`,"ts":`)
	if err := writeJSON(&b, event.TS); err != nil {
		return nil, err
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

func writeJSON(b *bytes.Buffer, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	b.Write(data)
	return nil
}
