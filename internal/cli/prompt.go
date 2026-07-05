// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-4.1, FR-5.3.
package cli

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/config"
)

// PromptBuilder renders producer and reviewer prompts.
type PromptBuilder struct {
	Repo  string
	TplFS fs.FS
	Vars  map[string]string
}

// Producer builds a producer prompt and includes prior reviewer notes.
func (b PromptBuilder) Producer(step config.Step, priorNotes []adapter.Verdict) (string, error) {
	body, err := b.render("producer.tmpl", step)
	if err != nil {
		return "", err
	}
	if len(priorNotes) == 0 {
		return body, nil
	}
	var notes strings.Builder
	notes.WriteString(body)
	notes.WriteString("\n\nPrior reviewer notes:\n")
	for _, verdict := range priorNotes {
		fmt.Fprintf(&notes, "- %s: %s\n", verdict.Adapter, verdict.Notes)
	}
	return notes.String(), nil
}

// Reviewer builds a strict JSON review prompt for an artifact.
func (b PromptBuilder) Reviewer(step config.Step, artifactPath string) (string, error) {
	body, err := b.render("reviewer.tmpl", step)
	if err != nil {
		return "", err
	}
	return body + "\nReview artifact: " + artifactPath + "\nReturn JSON only: {\"pass\":bool,\"severity\":\"low|medium|high|critical|error\",\"notes\":\"short\"}.", nil
}

func (b PromptBuilder) render(name string, step config.Step) (string, error) {
	raw, err := b.readPrompt(name)
	if err != nil {
		return "", err
	}
	tpl, err := template.New(name).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return "", err
	}
	data := map[string]any{"Step": step, "Vars": b.Vars}
	var out bytes.Buffer
	if err := tpl.Execute(&out, data); err != nil {
		return "", err
	}
	return out.String(), nil
}

func (b PromptBuilder) readPrompt(name string) ([]byte, error) {
	if b.Repo != "" {
		path := filepath.Join(b.Repo, ".ai", "workflows", "prompts", name)
		if data, err := os.ReadFile(path); err == nil {
			return data, nil
		}
	}
	if b.TplFS != nil {
		if data, err := fs.ReadFile(b.TplFS, filepath.ToSlash(filepath.Join("templates", "prompts", name))); err == nil {
			return data, nil
		}
	}
	switch name {
	case "producer.tmpl":
		return []byte("Objective: {{with index .Vars \"objective\"}}{{.}}{{else}}use the workflow step and repo context{{end}}\nProduce artifact {{.Step.Artifact}} for {{.Vars.project}}."), nil
	case "reviewer.tmpl":
		return []byte("{{with index .Vars \"objective\"}}Objective: {{.}}\n{{end}}Review {{.Step.Artifact}} for {{.Vars.project}}."), nil
	default:
		return nil, fmt.Errorf("prompt template %q not found", name)
	}
}
