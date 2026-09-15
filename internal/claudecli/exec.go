package claudecli

import (
	"context"
	"io"
	"os/exec"
)

// Run is the real Exec: no stdin, all output discarded, no console window on Windows.
func Run(ctx context.Context, dir, path string, args, env []string) error {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir, cmd.Env = dir, env
	cmd.Stdout, cmd.Stderr = io.Discard, io.Discard
	hideWindow(cmd)
	return cmd.Run()
}
