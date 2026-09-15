//go:build windows

package claudecli

import (
	"os/exec"
	"syscall"
)

// hideWindow keeps a console from flashing up when the GUI (built with -H=windowsgui)
// starts claude.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
}
