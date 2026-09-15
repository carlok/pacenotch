package gui

// Preference keys.
const (
	// PrefShowInDock (macOS): show pacenotch in the Dock and Cmd-Tab like a regular app,
	// instead of menu-bar-only. Useful when the camera notch hides the menu bar icon.
	PrefShowInDock = "showInDock"

	// PrefRefreshToken: when Claude Code's token has expired, run a tiny claude -p request
	// so Claude Code refreshes it. On by default.
	PrefRefreshToken = "refreshToken"
)
