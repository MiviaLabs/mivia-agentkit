// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"context"
	"fmt"
)

// RecordedCall is one fake runner invocation.
type RecordedCall struct {
	Args    []string
	Env     []string
	Workdir string
}

// FakeRunner scripts adapter process calls for tests.
type FakeRunner struct {
	Scripts map[string]FakeResponse
	Calls   []RecordedCall
}

// FakeResponse is a scripted fake process result.
type FakeResponse struct {
	Result RunResult
	Err    error
}

// Run records and returns a scripted response by command name.
func (f *FakeRunner) Run(ctx context.Context, args []string, env []string, workdir string) (RunResult, error) {
	f.Calls = append(f.Calls, RecordedCall{
		Args:    append([]string(nil), args...),
		Env:     append([]string(nil), env...),
		Workdir: workdir,
	})
	if len(args) == 0 {
		return RunResult{ExitCode: -1}, fmt.Errorf("missing command")
	}
	resp, ok := f.Scripts[args[0]]
	if !ok {
		return RunResult{ExitCode: 127}, fmt.Errorf("no fake response for %q", args[0])
	}
	return resp.Result, resp.Err
}
