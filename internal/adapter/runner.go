// Package adapter defines headless CLI adapter contracts.
// Plan: WS9. PRD: FR-3.1, FR-3.2, FR-7.4.
package adapter

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
)

const maxCapturedBytes = 1024 * 1024

// ErrCommandTimeout indicates the process context expired.
var ErrCommandTimeout = errors.New("adapter command timed out")

// Runner executes a local process.
type Runner interface {
	Run(ctx context.Context, args []string, env []string, workdir string) (RunResult, error)
}

// RunResult captures process output.
type RunResult struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
}

// OSRunner executes commands through the local OS.
type OSRunner struct{}

// Run executes args[0] with args[1:].
func (OSRunner) Run(ctx context.Context, args []string, env []string, workdir string) (RunResult, error) {
	if len(args) == 0 {
		return RunResult{ExitCode: -1}, errors.New("missing command")
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Dir = workdir
	cmd.Env = env
	var stdout, stderr limitedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := -1
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	}
	res := RunResult{ExitCode: exitCode, Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}
	if ctx.Err() != nil {
		res.ExitCode = -1
		return res, ErrCommandTimeout
	}
	if err != nil {
		return res, err
	}
	return res, nil
}

type limitedBuffer struct {
	buf bytes.Buffer
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	remaining := maxCapturedBytes - b.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			_, _ = b.buf.Write(p[:remaining])
		} else {
			_, _ = b.buf.Write(p)
		}
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}
