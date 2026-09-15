//go:build !windows

package claudecli

import "os/exec"

// hideWindow: only Windows opens console windows for child processes.
func hideWindow(*exec.Cmd) {}
