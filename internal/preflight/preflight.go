// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
	"github.com/MiviaLabs/mivia-agentkit/internal/pathpolicy"
)

const stampRelPath = ".git/mivia-agent-quality-stamp.json"

// Context configures a preflight run.
type Context struct {
	Repo              string
	ContractRows      []string
	FocusedVerifiers  []string
	BroadVerifiers    []string
	MutationProofs    []string
	NotRun            []string
	PipelinePreflight bool
}

// Run validates proof inputs and writes a fresh quality stamp.
func Run(ctx Context) (Stamp, error) {
	repo, err := gitstate.DetectRoot(defaultRepo(ctx.Repo))
	if err != nil {
		return Stamp{}, err
	}
	changed, err := gitstate.ChangedFiles(repo)
	if err != nil {
		return Stamp{}, err
	}
	changed, err = expandDirectoryEntries(repo, changed)
	if err != nil {
		return Stamp{}, err
	}
	head, err := gitstate.Head(repo)
	if err != nil {
		return Stamp{}, err
	}
	diff, err := gitstate.DiffHash(repo, changed)
	if err != nil {
		return Stamp{}, err
	}
	if err := validateProofs(changed, ctx); err != nil {
		return Stamp{}, err
	}
	stamp := NewStamp(head, diff, changed)
	stamp.ContractRows = sortedCopy(ctx.ContractRows)
	stamp.FocusedVerifiers = sortedCopy(ctx.FocusedVerifiers)
	stamp.BroadVerifiers = sortedCopy(ctx.BroadVerifiers)
	stamp.MutationProofs = sortedCopy(ctx.MutationProofs)
	stamp.NotRun = sortedCopy(ctx.NotRun)
	data, err := stamp.Marshal()
	if err != nil {
		return Stamp{}, err
	}
	path, err := stampPath(repo)
	if err != nil {
		return Stamp{}, err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return Stamp{}, fmt.Errorf("write quality stamp: %w", err)
	}
	return stamp, nil
}

func validateProofs(changed []string, ctx Context) error {
	var missing []string
	for _, reason := range ctx.NotRun {
		if strings.TrimSpace(reason) == "" {
			return fmt.Errorf("not-run reason must be non-empty")
		}
	}
	if len(ctx.NotRun) > 0 && len(ctx.BroadVerifiers) > 0 {
		return fmt.Errorf("not-run reason is only allowed when broad verifier is missing")
	}
	if Classify(changed, ContractMatrix{RequireContractRowsFor: ctx.ContractRows}) == High {
		if len(ctx.ContractRows) == 0 {
			missing = append(missing, "contract row")
		}
		if len(ctx.FocusedVerifiers) == 0 {
			missing = append(missing, "focused verifier")
		}
		if len(ctx.MutationProofs) == 0 {
			missing = append(missing, "mutation proof")
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required preflight proof: %s", strings.Join(missing, ", "))
	}
	return nil
}

func expandDirectoryEntries(repo string, files []string) ([]string, error) {
	expanded := make([]string, 0, len(files))
	for _, file := range files {
		path := filepath.Join(repo, filepath.FromSlash(file))
		info, err := os.Stat(path)
		if err != nil {
			expanded = append(expanded, filepath.ToSlash(file))
			continue
		}
		if !info.IsDir() {
			expanded = append(expanded, filepath.ToSlash(file))
			continue
		}
		err = filepath.WalkDir(path, func(child string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(repo, child)
			if err != nil {
				return err
			}
			expanded = append(expanded, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("expand directory %s: %w", file, err)
		}
	}
	sort.Strings(expanded)
	return expanded, nil
}

func stampPath(repo string) (string, error) {
	policy := pathpolicy.NewDefault()
	if err := policy.Check(repo, stampRelPath); err != nil {
		return "", err
	}
	path, err := policy.Abs(repo, stampRelPath)
	if err != nil {
		return "", err
	}
	if filepath.ToSlash(path) != filepath.ToSlash(filepath.Join(repo, stampRelPath)) {
		return "", fmt.Errorf("quality stamp must be under .git")
	}
	return path, nil
}

func defaultRepo(repo string) string {
	if repo != "" {
		return repo
	}
	return "."
}
