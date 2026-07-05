// Package preflight writes and validates quality stamps.
// Plan: WS4. PRD: FR-2.4, FR-7.1.
package preflight

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"github.com/MiviaLabs/mivia-agentkit/internal/gitstate"
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
func CheckStamp(repo string) (Stamp, error) {
	root, err := gitstate.DetectRoot(defaultRepo(repo))
	if err != nil {
		return Stamp{}, err
	}
	path, err := stampPath(root)
	if err != nil {
		return Stamp{}, err
	}
	data, err := os.ReadFile(path)
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
	head, err := gitstate.Head(root)
	if err != nil {
		return Stamp{}, err
	}
	changed, err := gitstate.ChangedFiles(root)
	if err != nil {
		return Stamp{}, err
	}
	changed, err = expandDirectoryEntries(root, changed)
	if err != nil {
		return Stamp{}, err
	}
	diff, err := gitstate.DiffHash(root, changed)
	if err != nil {
		return Stamp{}, err
	}
	if head != stamp.Head {
		return Stamp{}, ErrStaleStamp{Reason: "head changed"}
	}
	if diff != stamp.DiffSHA256 {
		return Stamp{}, ErrStaleStamp{Reason: "diff hash changed"}
	}
	if !slices.Equal(sortedCopy(changed), sortedCopy(stamp.ChangedFiles)) {
		return Stamp{}, ErrStaleStamp{Reason: "changed files changed"}
	}
	return stamp, nil
}
