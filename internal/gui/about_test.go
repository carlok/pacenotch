package gui

import (
	"strings"
	"testing"
)

func TestWrap(t *testing.T) {
	tests := []struct {
		in    string
		width int
		want  []string
	}{
		{"", 10, nil},
		{"   ", 10, nil},
		{"one two three", 7, []string{"one two", "three"}},
		{"one two three", 13, []string{"one two three"}},
		{"a supercalifragilistic word", 6, []string{"a", "supercalifragilistic", "word"}},
		{"né più né meno", 6, []string{"né più", "né", "meno"}},
	}
	for _, tt := range tests {
		got := Wrap(tt.in, tt.width)
		if strings.Join(got, "|") != strings.Join(tt.want, "|") || len(got) != len(tt.want) {
			t.Errorf("Wrap(%q, %d) = %q, want %q", tt.in, tt.width, got, tt.want)
		}
	}
}

func TestAboutLines(t *testing.T) {
	lines := AboutLines("v1.2.3")
	if lines[0] != "pacenotch v1.2.3" || !strings.Contains(strings.Join(lines, "\n"), "Not affiliated with, or endorsed by, Anthropic") {
		t.Errorf("about: %q", lines)
	}
	if AboutLines("")[0] != "pacenotch dev" {
		t.Error("empty version")
	}
	if !strings.HasPrefix(IssuesURL, "https://github.com/carlok/pacenotch") {
		t.Error("repo URL")
	}
}
