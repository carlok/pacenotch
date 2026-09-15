// Package autostart turns "start pacenotch at login" on and off: a LaunchAgent on macOS, an
// XDG autostart entry on Linux, a Run registry value on Windows. The OS entry is the source
// of truth, so the setting can never disagree with what actually starts.
package autostart

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Label is the LaunchAgent label.
const Label = "io.github.carlok.pacenotch"

// RunValue is the name of the Windows Run registry value.
const RunValue = "pacenotch"

// ErrUnsupported is returned on systems without a known login mechanism.
var ErrUnsupported = errors.New("start at login is not supported on this system")

// Registry is the current user's Windows Run key.
type Registry interface {
	Get(name string) (string, error) // fs.ErrNotExist when the value is missing
	Set(name, value string) error
	Delete(name string) error // no error when the value is missing
}

// System holds every side effect, so tests can replace them.
type System struct {
	GOOS       string
	HomeDir    func() (string, error)
	Getenv     func(string) string
	Executable func() (string, error)
	ReadFile   func(string) ([]byte, error)
	WriteFile  func(string, []byte, fs.FileMode) error
	MkdirAll   func(string, fs.FileMode) error
	Remove     func(string) error
	Registry   Registry // Windows only
}

// Default uses the real system.
func Default() System {
	return System{
		GOOS: runtime.GOOS, HomeDir: os.UserHomeDir, Getenv: os.Getenv, Executable: os.Executable,
		ReadFile: os.ReadFile, WriteFile: os.WriteFile, MkdirAll: os.MkdirAll, Remove: os.Remove,
		Registry: defaultRegistry(),
	}
}

// Command is what runs at login for the executable exe: the .app bundle through open on
// macOS, pacenotch-gui.exe as it is on Windows, otherwise `exe gui`.
func Command(goos, exe string) []string {
	if goos == "darwin" {
		if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
			return []string{"/usr/bin/open", "-a", exe[:i+len(".app")]}
		}
	}
	if goos == "windows" && strings.HasPrefix(strings.ToLower(exe[strings.LastIndexAny(exe, `/\`)+1:]), "pacenotch-gui") {
		return []string{exe}
	}
	return []string{exe, "gui"}
}

// LaunchAgentPlist is the macOS LaunchAgent that runs args at login.
func LaunchAgentPlist(args []string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
		`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n" +
		`<plist version="1.0">` + "\n<dict>\n\t<key>Label</key>\n\t<string>" + Label +
		"</string>\n\t<key>ProgramArguments</key>\n\t<array>\n")
	for _, a := range args {
		b.WriteString("\t\t<string>")
		xml.EscapeText(&b, []byte(a))
		b.WriteString("</string>\n")
	}
	b.WriteString("\t</array>\n\t<key>RunAtLoad</key>\n\t<true/>\n</dict>\n</plist>\n")
	return b.Bytes()
}

// DesktopEntry is the XDG autostart entry that runs args at login.
func DesktopEntry(args []string) []byte {
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = desktopQuote(a)
	}
	return []byte("[Desktop Entry]\nType=Application\nName=pacenotch\n" +
		"Comment=Claude usage limits with a vertical pace notch\nExec=" + strings.Join(quoted, " ") +
		"\nTerminal=false\nX-GNOME-Autostart-enabled=true\n")
}

// desktopQuote quotes an Exec argument as the Desktop Entry specification requires.
func desktopQuote(a string) string {
	a = strings.ReplaceAll(a, "%", "%%")
	if a != "" && !strings.ContainsAny(a, " \t\n\"'\\><~|&;$*?#()`") {
		return a
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", `$`, `\$`).Replace(a) + `"`
}

// WindowsCommandLine joins args for the Run registry value.
func WindowsCommandLine(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\"") {
			parts[i] = a
			continue
		}
		parts[i] = `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
	}
	return strings.Join(parts, " ")
}

// file is where the entry lives on macOS and Linux, and how to render it.
func (s System) file() (string, func([]string) []byte, error) {
	home, herr := s.HomeDir()
	switch s.GOOS {
	case "darwin":
		if herr != nil {
			return "", nil, herr
		}
		return filepath.Join(home, "Library", "LaunchAgents", Label+".plist"), LaunchAgentPlist, nil
	case "linux", "freebsd", "openbsd", "netbsd":
		dir := s.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			if herr != nil {
				return "", nil, herr
			}
			dir = filepath.Join(home, ".config")
		}
		return filepath.Join(dir, "autostart", "pacenotch.desktop"), DesktopEntry, nil
	}
	return "", nil, ErrUnsupported
}

func (s System) registry() (Registry, error) {
	if s.Registry == nil {
		return nil, ErrUnsupported
	}
	return s.Registry, nil
}

// wanted is the entry content for the running executable.
func (s System) wanted() ([]string, error) {
	exe, err := s.Executable()
	if err != nil {
		return nil, err
	}
	return Command(s.GOOS, exe), nil
}

// Enabled reports whether pacenotch starts at login.
func (s System) Enabled() (bool, error) {
	var err error
	if s.GOOS == "windows" {
		var reg Registry
		if reg, err = s.registry(); err == nil {
			_, err = reg.Get(RunValue)
		}
	} else {
		var path string
		if path, _, err = s.file(); err == nil {
			_, err = s.ReadFile(path)
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Enable makes the running executable start at login.
func (s System) Enable() error {
	return s.write(false)
}

// Repair rewrites an existing entry that no longer points at the running executable, for
// example after the app was moved. It does nothing when start at login is off.
func (s System) Repair() error {
	return s.write(true)
}

func (s System) write(onlyIfChanged bool) error {
	args, err := s.wanted()
	if err != nil {
		return err
	}
	if s.GOOS == "windows" {
		reg, err := s.registry()
		if err != nil {
			return err
		}
		line := WindowsCommandLine(args)
		if onlyIfChanged {
			cur, err := reg.Get(RunValue)
			if errors.Is(err, fs.ErrNotExist) || (err == nil && cur == line) {
				return nil
			}
			if err != nil {
				return err
			}
		}
		return reg.Set(RunValue, line)
	}
	path, render, err := s.file()
	if err != nil {
		return err
	}
	content := render(args)
	if onlyIfChanged {
		cur, err := s.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) || (err == nil && bytes.Equal(cur, content)) {
			return nil
		}
		if err != nil {
			return err
		}
	}
	if err := s.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return s.WriteFile(path, content, 0o644)
}

// Disable stops pacenotch from starting at login. It is not an error if it was off.
func (s System) Disable() error {
	if s.GOOS == "windows" {
		reg, err := s.registry()
		if err != nil {
			return err
		}
		return reg.Delete(RunValue)
	}
	path, _, err := s.file()
	if err != nil {
		return err
	}
	if err := s.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
