// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-5.3.
package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
)

func TestReviewOneOffConsensus(t *testing.T) {
	t.Run("pass", func(t *testing.T) {
		repo, artifactPath := reviewRepo(t)
		withRuntimeAdapters(t,
			fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
			fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}},
		)
		cmd := newReviewCommand()
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex,claude", "--mode", "majority", "--min-reviewers", "2", "--json"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("review error = %v", err)
		}
		if !containsAll(out.String(), "consensus passed", "codex", "claude") {
			t.Fatalf("review output = %s, want consensus verdicts", out.String())
		}
	})

	t.Run("fail", func(t *testing.T) {
		repo, artifactPath := reviewRepo(t)
		withRuntimeAdapters(t,
			fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
			fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
		)
		cmd := newReviewCommand()
		cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex,claude", "--mode", "majority", "--min-reviewers", "2"})
		if err := cmd.Execute(); err == nil {
			t.Fatalf("review error = nil, want consensus failure")
		}
	})
}

func TestReviewRespectsWeights(t *testing.T) {
	repo, artifactPath := reviewRepo(t)
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: false, Severity: "high", Notes: "no"}},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "yes"}},
	)
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex,claude", "--mode", "weighted", "--min-reviewers", "2", "--weights", "claude=2,codex=1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("review error = %v, want weighted pass", err)
	}
}

func TestReviewResolvesArtifactRelativeToRepo(t *testing.T) {
	repo := t.TempDir()
	mustWrite(t, filepath.Join(repo, "artifact.md"), "review me")
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}})
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", "artifact.md", "--reviewers", "codex", "--min-reviewers", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("review error = %v, want repo-relative artifact path to resolve", err)
	}
}

func TestReviewPassesManifestAdapterDefaultsToRuntime(t *testing.T) {
	var reviewReqs []adapter.Request
	repo, artifactPath := reviewRepo(t)
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nadapters:\n  codex:\n    enabled: true\n    role: orchestrable\n    model: gpt-5.5\n    effort: minimal\n")
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, reviews: &reviewReqs})
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex", "--min-reviewers", "1"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("review error = %v", err)
	}
	if len(reviewReqs) != 1 || reviewReqs[0].Model != "gpt-5.5" || reviewReqs[0].Effort != "minimal" {
		t.Fatalf("review requests = %#v, want manifest defaults", reviewReqs)
	}
}

func TestReviewUsesRenderedAdapterScopedDefaults(t *testing.T) {
	var reviewReqs []adapter.Request
	t.Setenv("HOME", t.TempDir())
	repo, artifactPath := reviewRepo(t)
	if _, err := render.WriteInit(render.InitConfig{Repo: repo, Profile: "standard", Adapters: []string{"codex"}}); err != nil {
		t.Fatalf("WriteInit() error = %v, want nil", err)
	}
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}, reviews: &reviewReqs})
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("review error = %v, want codex-only rendered defaults to pass", err)
	}
	if len(reviewReqs) != 1 {
		t.Fatalf("review requests = %#v, want one default codex reviewer", reviewReqs)
	}
}

func TestReviewRejectsMinReviewersUnsatisfied(t *testing.T) {
	repo, artifactPath := reviewRepo(t)
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true, Severity: "low", Notes: "ok"}})
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex", "--mode", "majority", "--min-reviewers", "2"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("review error = nil, want min reviewers rejection")
	}
}

func TestReviewPropagatesCommandContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo, artifactPath := reviewRepo(t)
	withRuntimeAdapters(t, contextAwareAdapter{name: "codex"})
	cmd := newReviewCommand()
	cmd.SetContext(ctx)
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex", "--min-reviewers", "1"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("review error = nil, want canceled context error")
	}
}

func TestReviewArtifactMustExist(t *testing.T) {
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", t.TempDir(), "--artifact", "missing.md", "--reviewers", "codex"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("review error = nil, want missing artifact rejection")
	}
}

func reviewRepo(t *testing.T) (string, string) {
	t.Helper()
	repo := t.TempDir()
	artifactPath := filepath.Join(repo, "artifact.md")
	mustWrite(t, artifactPath, "review me")
	return repo, artifactPath
}
