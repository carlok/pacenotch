package autostart

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

type fakeFS struct {
	files                         map[string][]byte
	dirs                          []string
	exe                           string
	home                          string
	vars                          map[string]string
	homeErr, exeErr, readErr      error
	writeErr, mkdirErr, removeErr error
}

func (f *fakeFS) system(goos string, reg Registry) System {
	return System{
		GOOS: goos,
		HomeDir: func() (string, error) {
			return f.home, f.homeErr
		},
		Getenv:     func(k string) string { return f.vars[k] },
		Executable: func() (string, error) { return f.exe, f.exeErr },
		ReadFile: func(p string) ([]byte, error) {
			if f.readErr != nil {
				return nil, f.readErr
			}
			if b, ok := f.files[p]; ok {
				return b, nil
			}
			return nil, fs.ErrNotExist
		},
		WriteFile: func(p string, b []byte, _ fs.FileMode) error {
			if f.writeErr != nil {
				return f.writeErr
			}
			f.files[p] = b
			return nil
		},
		MkdirAll: func(p string, _ fs.FileMode) error {
			f.dirs = append(f.dirs, p)
			return f.mkdirErr
		},
		Remove: func(p string) error {
			if f.removeErr != nil {
				return f.removeErr
			}
			if _, ok := f.files[p]; !ok {
				return fs.ErrNotExist
			}
			delete(f.files, p)
			return nil
		},
		Registry: reg,
	}
}

type fakeReg struct {
	values map[string]string
	getErr error
}

func (r *fakeReg) Get(name string) (string, error) {
	if r.getErr != nil {
		return "", r.getErr
	}
	v, ok := r.values[name]
	if !ok {
		return "", fs.ErrNotExist
	}
	return v, nil
}
func (r *fakeReg) Set(name, value string) error { r.values[name] = value; return nil }
func (r *fakeReg) Delete(name string) error     { delete(r.values, name); return nil }

func TestCommand(t *testing.T) {
	tests := []struct {
		goos, exe string
		want      []string
	}{
		{"darwin", "/Applications/pacenotch.app/Contents/MacOS/pacenotch", []string{"/usr/bin/open", "-a", "/Applications/pacenotch.app"}},
		{"darwin", "/usr/local/bin/pacenotch", []string{"/usr/local/bin/pacenotch", "gui"}},
		{"linux", "/opt/p/pacenotch-gui_linux_amd64", []string{"/opt/p/pacenotch-gui_linux_amd64", "gui"}},
		{"windows", `C:\Tools\Pacenotch-GUI_windows_amd64.exe`, []string{`C:\Tools\Pacenotch-GUI_windows_amd64.exe`}},
		{"windows", `C:\Tools\pacenotch.exe`, []string{`C:\Tools\pacenotch.exe`, "gui"}},
	}
	for _, tt := range tests {
		if got := Command(tt.goos, tt.exe); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Command(%s, %s) = %q, want %q", tt.goos, tt.exe, got, tt.want)
		}
	}
}

func TestRendering(t *testing.T) {
	plist := string(LaunchAgentPlist([]string{"/usr/bin/open", "-a", "/Apps/R&D <x>.app"}))
	for _, want := range []string{"<string>io.github.carlok.pacenotch</string>", "<string>/usr/bin/open</string>",
		"<string>/Apps/R&amp;D &lt;x&gt;.app</string>", "<key>RunAtLoad</key>\n\t<true/>"} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist is missing %q:\n%s", want, plist)
		}
	}
	entry := string(DesktopEntry([]string{"/home/u/My Apps/pace$notch", "gui", "100%"}))
	if !strings.Contains(entry, `Exec="/home/u/My Apps/pace\$notch" gui 100%%`+"\n") || !strings.HasPrefix(entry, "[Desktop Entry]\n") {
		t.Errorf("desktop entry:\n%s", entry)
	}
	if got := desktopQuote(""); got != `""` {
		t.Errorf("empty argument %q", got)
	}
	if got := WindowsCommandLine([]string{`C:\Program Files\p\pacenotch.exe`, "gui", `a"b`, ""}); got != `"C:\Program Files\p\pacenotch.exe" gui "a\"b" ""` {
		t.Errorf("windows command line %q", got)
	}
}

// TestPlistIsValid checks the LaunchAgent with Apple's own linter where it exists.
func TestPlistIsValid(t *testing.T) {
	plutil, err := exec.LookPath("plutil")
	if err != nil {
		t.Skip("plutil is macOS only")
	}
	path := filepath.Join(t.TempDir(), Label+".plist")
	os.WriteFile(path, LaunchAgentPlist(Command("darwin", "/Applications/R&D <x>.app/Contents/MacOS/pacenotch")), 0o644)
	if out, err := exec.Command(plutil, "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil -lint: %v\n%s", err, out)
	}
}

func TestUnixEntries(t *testing.T) {
	for _, tt := range []struct {
		goos, want string
		vars       map[string]string
	}{
		{"darwin", filepath.Join("/Users/u", "Library", "LaunchAgents", Label+".plist"), nil},
		{"linux", filepath.Join("/Users/u", ".config", "autostart", "pacenotch.desktop"), nil},
		{"freebsd", filepath.Join("/xdg", "autostart", "pacenotch.desktop"), map[string]string{"XDG_CONFIG_HOME": "/xdg"}},
	} {
		f := &fakeFS{files: map[string][]byte{}, home: "/Users/u", exe: "/Applications/pacenotch.app/Contents/MacOS/pacenotch", vars: tt.vars}
		s := f.system(tt.goos, nil)
		if on, err := s.Enabled(); on || err != nil {
			t.Fatalf("%s: enabled before enabling: %v %v", tt.goos, on, err)
		}
		if err := s.Repair(); err != nil || len(f.files) != 0 {
			t.Fatalf("%s: repair must not create an entry: %v", tt.goos, err)
		}
		if err := s.Enable(); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.files[tt.want]; !ok || f.dirs[0] != filepath.Dir(tt.want) {
			t.Fatalf("%s: files %v dirs %v", tt.goos, f.files, f.dirs)
		}
		if on, err := s.Enabled(); !on || err != nil {
			t.Fatalf("%s: enabled: %v %v", tt.goos, on, err)
		}
		before := string(f.files[tt.want])
		f.exe = "/Other/pacenotch.app/Contents/MacOS/pacenotch"
		if err := s.Repair(); err != nil || string(f.files[tt.want]) == before {
			t.Fatalf("%s: a moved app must be repaired: %v", tt.goos, err)
		}
		f.dirs = nil
		if err := s.Repair(); err != nil || f.dirs != nil {
			t.Fatalf("%s: nothing to repair: %v %v", tt.goos, err, f.dirs)
		}
		if err := s.Disable(); err != nil {
			t.Fatal(err)
		}
		if err := s.Disable(); err != nil {
			t.Fatalf("%s: disabling twice: %v", tt.goos, err)
		}
		if on, _ := s.Enabled(); on {
			t.Fatalf("%s: still enabled", tt.goos)
		}
	}
}

func TestWindowsEntry(t *testing.T) {
	f := &fakeFS{exe: `C:\p\pacenotch-gui_windows_amd64.exe`}
	reg := &fakeReg{values: map[string]string{}}
	s := f.system("windows", reg)
	if on, err := s.Enabled(); on || err != nil {
		t.Fatal(on, err)
	}
	if err := s.Repair(); err != nil || len(reg.values) != 0 {
		t.Fatalf("repair must not create an entry: %v", err)
	}
	if err := s.Enable(); err != nil || reg.values[RunValue] != `C:\p\pacenotch-gui_windows_amd64.exe` {
		t.Fatalf("enable: %v %v", err, reg.values)
	}
	f.exe = `C:\Program Files\pacenotch\pacenotch-gui.exe`
	if err := s.Repair(); err != nil || reg.values[RunValue] != `"C:\Program Files\pacenotch\pacenotch-gui.exe"` {
		t.Fatalf("repair: %v %v", err, reg.values)
	}
	if err := s.Repair(); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Enabled(); !on {
		t.Fatal("enabled")
	}
	if err := s.Disable(); err != nil || len(reg.values) != 0 {
		t.Fatal(err)
	}
	reg.getErr = errors.New("access denied")
	if on, err := s.Enabled(); on || err == nil {
		t.Error("registry errors are reported")
	}
	if err := s.Repair(); err == nil {
		t.Error("repair reports registry errors")
	}
}

func TestErrors(t *testing.T) {
	boom := errors.New("boom")
	cases := []struct {
		name  string
		setup func(*fakeFS)
		goos  string
		reg   Registry
		op    func(System) error
	}{
		{"unsupported enable", nil, "plan9", nil, System.Enable},
		{"unsupported disable", nil, "plan9", nil, System.Disable},
		{"windows without registry", nil, "windows", nil, System.Enable},
		{"windows without registry, disable", nil, "windows", nil, System.Disable},
		{"windows without registry, repair", nil, "windows", nil, System.Repair},
		{"no home", func(f *fakeFS) { f.homeErr = boom }, "darwin", nil, System.Enable},
		{"no home on linux", func(f *fakeFS) { f.homeErr = boom }, "linux", nil, System.Disable},
		{"no executable", func(f *fakeFS) { f.exeErr = boom }, "linux", nil, System.Enable},
		{"mkdir", func(f *fakeFS) { f.mkdirErr = boom }, "linux", nil, System.Enable},
		{"write", func(f *fakeFS) { f.writeErr = boom }, "darwin", nil, System.Enable},
		{"remove", func(f *fakeFS) { f.removeErr = boom }, "darwin", nil, System.Disable},
		{"read during repair", func(f *fakeFS) { f.readErr = boom }, "darwin", nil, System.Repair},
		{"file paths during repair", func(f *fakeFS) { f.homeErr = boom }, "darwin", nil, System.Repair},
	}
	for _, c := range cases {
		f := &fakeFS{files: map[string][]byte{}, home: "/h", exe: "/bin/pacenotch"}
		if c.setup != nil {
			c.setup(f)
		}
		if err := c.op(f.system(c.goos, c.reg)); err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
	}
	f := &fakeFS{files: map[string][]byte{}, home: "/h", readErr: boom}
	if on, err := f.system("linux", nil).Enabled(); on || !errors.Is(err, boom) {
		t.Errorf("read errors are reported: %v %v", on, err)
	}
	if on, err := (&fakeFS{}).system("plan9", nil).Enabled(); on || !errors.Is(err, ErrUnsupported) {
		t.Errorf("unsupported: %v %v", on, err)
	}
	if on, err := (&fakeFS{}).system("windows", nil).Enabled(); on || !errors.Is(err, ErrUnsupported) {
		t.Errorf("windows without registry: %v %v", on, err)
	}
}

func TestDefault(t *testing.T) {
	s := Default()
	if s.GOOS != runtime.GOOS || s.HomeDir == nil || s.Executable == nil || s.WriteFile == nil || s.Remove == nil {
		t.Fatalf("%+v", s)
	}
	if (s.Registry != nil) != (runtime.GOOS == "windows") {
		t.Error("the registry is Windows only")
	}
}
