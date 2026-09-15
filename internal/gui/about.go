package gui

import (
	"strings"
	"unicode/utf8"
)

// Wrap breaks text into lines of at most width runes at spaces. A word longer than width
// gets a line of its own.
func Wrap(text string, width int) []string {
	var lines []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case utf8.RuneCountInString(line)+1+utf8.RuneCountInString(word) > width:
			lines = append(lines, line)
			line = word
		default:
			line += " " + word
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

// About box and credits.
const (
	AppID      = "io.github.carlok.pacenotch"
	RepoURL    = "https://github.com/carlok/pacenotch"
	IssuesURL  = RepoURL + "/issues"
	Tagline    = "Claude usage limits with a vertical pace notch"
	AboutNotch = "The notch marks where your usage would be if you spent the window evenly. " +
		"Fill past the notch means you are ahead of pace: slow down. Fill short of it means room to spare."
	Disclaimer = "Unofficial. Not affiliated with, or endorsed by, Anthropic."
	Copyright  = "© 2026 Carlo Perassi · MIT License"
	Credits    = "Built with Go and Fyne. Reads the usage limits with the token Claude Code stores, " +
		"never writing it; when that token has expired, it can ask Claude Code to refresh it " +
		"with a one-line request. The usage endpoint is undocumented and may change."
)

// AboutLines is the plain-text content of the About window, top to bottom.
func AboutLines(version string) []string {
	if version == "" {
		version = "dev"
	}
	return []string{"pacenotch " + version, Tagline, AboutNotch, Disclaimer, Credits, Copyright}
}
