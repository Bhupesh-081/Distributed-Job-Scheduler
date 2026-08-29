// Package executor runs a job's payload. MVP payload type is a shell
// command: {"cmd": "...", "args": [...], "timeout_seconds": N}. Args are
// passed straight to exec.Command as a slice, never through a shell, so a
// malicious payload can't break out via shell metacharacters; it can still
// run whatever binary "cmd" names, which is the tradeoff the user chose for
// this job type over a safer sandboxed one.
package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

const (
	defaultTimeout = 30 * time.Second
	maxTimeout     = 5 * time.Minute
)

type ShellPayload struct {
	Cmd            string   `json:"cmd"`
	Args           []string `json:"args"`
	TimeoutSeconds int      `json:"timeout_seconds"`
}

func Run(ctx context.Context, payload json.RawMessage) (string, error) {
	var p ShellPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		return "", fmt.Errorf("invalid payload: %w", err)
	}
	if p.Cmd == "" {
		return "", fmt.Errorf("payload.cmd is required")
	}

	timeout := defaultTimeout
	if p.TimeoutSeconds > 0 {
		timeout = time.Duration(p.TimeoutSeconds) * time.Second
	}
	if timeout > maxTimeout {
		timeout = maxTimeout
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	out, err := exec.CommandContext(ctx, p.Cmd, p.Args...).CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("timed out after %s", timeout)
	}
	if err != nil {
		return string(out), fmt.Errorf("exec: %w", err)
	}
	return string(out), nil
}
