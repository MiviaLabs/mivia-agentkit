// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MiviaLabs/mivia-agentkit/internal/config"
	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

// ErrNoStamp reports that no quality stamp exists.
var ErrNoStamp = errors.New("quality stamp missing")

// ErrStaleStamp reports that a quality stamp no longer matches repository state.
type ErrStaleStamp struct {
	Reason string
}

// Error returns the stale stamp reason.
func (e ErrStaleStamp) Error() string {
	return "quality stamp stale: " + e.Reason
}

// CheckStamp returns the stamp only when it matches current repository state.
func CheckStamp(repo string) (Stamp, error) { return checkStamp(repo, "") }

// CheckStampForRef accepts a stamp only when it is promoted to ref's exact tip.
func CheckStampForRef(repo, ref string) (Stamp, error) { return checkStamp(repo, ref) }

func checkStamp(repo, ref string) (Stamp, error) {
	root, err := gitstate.DetectRoot(defaultRepo(repo))
	if err != nil {
		return Stamp{}, err
	}
	stampRoot, _, err := stampLocation(root)
	if err != nil {
		return Stamp{}, err
	}
	data, err := pathpolicy.ReadFile(stampRoot, stampName)
	if err != nil {
		if os.IsNotExist(err) {
			return Stamp{}, ErrNoStamp
		}
		return Stamp{}, fmt.Errorf("read quality stamp: %w", err)
	}
	stamp, err := ParseStamp(data)
	if err != nil {
		return Stamp{}, err
	}
	if err := validateEvidence(root, stamp); err != nil {
		return Stamp{}, ErrStaleStamp{Reason: err.Error()}
	}
	head, err := gitstate.Head(root)
	if err != nil {
		return Stamp{}, err
	}
	if ref != "" {
		resolved, err := gitstate.ResolveRef(root, ref)
		if err != nil {
			return Stamp{}, err
		}
		head = resolved
	}
	if stamp.Subject.Commit != "" {
		if head != stamp.Subject.Commit {
			return Stamp{}, ErrStaleStamp{Reason: "validated commit is not HEAD"}
		}
		return stamp, validateCommitSubject(root, stamp)
	}
	indexTree, err := gitstate.IndexTree(root)
	if err != nil {
		return Stamp{}, err
	}
	if head == stamp.Subject.BaseHead && indexTree == stamp.Subject.IndexTree {
		return stamp, nil
	}
	if err := validateCommitCandidate(root, stamp, head); err != nil {
		return Stamp{}, ErrStaleStamp{Reason: err.Error()}
	}
	stamp.Subject.Commit = head
	stamp.Head = head
	data, err = stamp.Marshal()
	if err != nil {
		return Stamp{}, err
	}
	if err := pathpolicy.WriteFile(stampRoot, stampName, data, 0o600); err != nil {
		return Stamp{}, fmt.Errorf("promote quality stamp: %w", err)
	}
	return stamp, nil
}

func validateEvidence(repo string, stamp Stamp) error {
	if len(stamp.NotRun) != 0 || len(stamp.VerifierEvidence) == 0 {
		return fmt.Errorf("successful verifier evidence required")
	}
	expected := subjectHash(stamp.Subject)
	manifest, err := loadQuality(repo)
	if err != nil {
		return err
	}
	if len(manifest.RequiredVerifiers) == 0 {
		return fmt.Errorf("manifest declares no required verifier")
	}
	if len(stamp.VerifierEvidence) != len(manifest.RequiredVerifiers) {
		return fmt.Errorf("incomplete verifier evidence")
	}
	seen := map[string]struct{}{}
	now := time.Now().UTC()
	for i, id := range manifest.RequiredVerifiers {
		evidence := stamp.VerifierEvidence[i]
		definition, ok := manifest.Verifiers[id]
		started, startErr := time.Parse(time.RFC3339, evidence.StartedAt)
		finished, finishErr := time.Parse(time.RFC3339, evidence.FinishedAt)
		if _, duplicate := seen[evidence.CommandID]; duplicate || !ok || evidence.SchemaVersion != 1 || evidence.CommandID != id || evidence.DefinitionHash != verifierHash(definition) || evidence.SubjectHash != expected || evidence.ExitCode != 0 || evidence.Source != "local" || startErr != nil || finishErr != nil || started.After(now) || finished.After(now) || finished.Before(started) || now.Sub(finished) > 24*time.Hour {
			return fmt.Errorf("incomplete verifier evidence")
		}
		seen[evidence.CommandID] = struct{}{}
	}
	return nil
}

func loadQuality(repo string) (config.Quality, error) {
	data, err := os.ReadFile(filepath.Join(repo, "mivia-agent.yaml"))
	if err != nil {
		return config.Quality{}, fmt.Errorf("read manifest quality: %w", err)
	}
	manifest, err := config.Parse(data)
	if err != nil {
		return config.Quality{}, err
	}
	return manifest.Quality, nil
}

func validateCommitCandidate(repo string, stamp Stamp, head string) error {
	parent, err := gitstate.CommitParent(repo, head)
	if err != nil {
		return err
	}
	if parent != stamp.Subject.BaseHead {
		return fmt.Errorf("commit parent changed")
	}
	tree, err := gitstate.CommitTree(repo, head)
	if err != nil {
		return err
	}
	if tree != stamp.Subject.IndexTree {
		return fmt.Errorf("commit tree changed")
	}
	return nil
}

func validateCommitSubject(repo string, stamp Stamp) error {
	return validateCommitCandidate(repo, stamp, stamp.Subject.Commit)
}
