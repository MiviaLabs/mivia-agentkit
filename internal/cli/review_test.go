// Package cli implements the mivia-agent command surface.
// Plan: WS13. PRD: FR-5.3.
package cli

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MiviaLabs/mivia-agentkit/internal/adapter"
	"github.com/MiviaLabs/mivia-agentkit/internal/render"
)

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("output unavailable") }

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

func TestReviewUsesManifestWeights(t *testing.T) {
	repo, artifactPath := reviewRepo(t)
	mustWrite(t, filepath.Join(repo, "mivia-agent.yaml"), "version: \"1\"\nrouting:\n  consensus:\n    mode: weighted\n    min_reviewers: 2\n    weights: {codex: 3, claude: 1}\nadapters:\n  codex: {enabled: true, role: orchestrable}\n  claude: {enabled: true, role: orchestrable}\n")
	withRuntimeAdapters(t,
		fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: false}},
		fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: true}},
	)
	cmd := newReviewCommand()
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex,claude"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "review failed") {
		t.Fatalf("review error = %v, want manifest weighted failure distinct from default equal weights", err)
	}
}

func TestReviewRejectsMalformedWeights(t *testing.T) {
	for _, weights := range []string{"codex=nope", "codex=NaN", "codex=0", "codex=1,codex=2"} {
		t.Run(weights, func(t *testing.T) {
			repo, artifactPath := reviewRepo(t)
			cmd := newReviewCommand()
			cmd.SetArgs([]string{"--repo", filepath.Join(repo, "missing"), "--artifact", artifactPath, "--weights", weights})
			if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "invalid --weights") {
				t.Fatalf("review error = %v, want exact malformed weights rejection before repository access", err)
			}
		})
	}
}

func TestReviewPropagatesJSONWriteError(t *testing.T) {
	repo, artifactPath := reviewRepo(t)
	withRuntimeAdapters(t, fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true}})
	cmd := newReviewCommand()
	cmd.SetOut(failingWriter{})
	cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex", "--min-reviewers", "1", "--json"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "output unavailable") {
		t.Fatalf("review error = %v, want JSON write failure", err)
	}
}

func TestReviewTieBreakers(t *testing.T) {
	for _, tt := range []struct {
		name       string
		tieBreaker string
		wantPass   bool
	}{
		{name: "strict", tieBreaker: "strict", wantPass: false},
		{name: "manual", tieBreaker: "manual", wantPass: false},
		{name: "prefer passing reviewer", tieBreaker: "prefer:codex", wantPass: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo, artifactPath := reviewRepo(t)
			withRuntimeAdapters(t,
				fakeCLIAdapter{name: "codex", headless: true, verdict: adapter.Verdict{Pass: true}},
				fakeCLIAdapter{name: "claude", headless: true, verdict: adapter.Verdict{Pass: false}},
			)
			cmd := newReviewCommand()
			cmd.SetArgs([]string{"--repo", repo, "--artifact", artifactPath, "--reviewers", "codex,claude", "--mode", "majority", "--min-reviewers", "2", "--tie-breaker", tt.tieBreaker})
			err := cmd.Execute()
			if (err == nil) != tt.wantPass {
				t.Fatalf("review error = %v, want pass=%t", err, tt.wantPass)
			}
		})
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
