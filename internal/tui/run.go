package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/carlok/pacenotch/internal/claudecli"
	"github.com/carlok/pacenotch/internal/pace"
	"github.com/carlok/pacenotch/internal/usage"
)

// Program is the name printed in the header and in error messages.
const Program = "pacenotch"

// NowEnv fixes "now" (Unix seconds) for tests.
const NowEnv = "PACENOTCH_NOW"

// LoaderFunc builds the data loader; tests replace it to avoid the network.
type LoaderFunc func(from string, stdin io.Reader, ttl time.Duration, now func() time.Time) (*usage.Loader, error)

// Deps are the side effects of the CLI. DefaultDeps wires the real ones.
type Deps struct {
	Ctx        context.Context
	Args       []string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	Getenv     func(string) string
	Loc        *time.Location
	IsTerminal func() bool
	EnableVT   func() bool
	Size       func() (int, error)
	Resize     func(ctx context.Context, size func() (int, error)) <-chan struct{}
	After      func(time.Duration) <-chan time.Time
	NewLoader  LoaderFunc
	Refresh    func(ctx context.Context) error // asks Claude Code to refresh an expired token
	Version    string
}

// RefreshEnv set to 1 turns on --refresh-token.
const RefreshEnv = "PACENOTCH_REFRESH"

// DefaultDeps uses the process's stdio, environment, terminal and clock.
func DefaultDeps(ctx context.Context, args []string, version string) Deps {
	return Deps{
		Ctx: ctx, Args: args, Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr,
		Getenv:     os.Getenv,
		Loc:        time.Local,
		IsTerminal: func() bool { return term.IsTerminal(int(os.Stdout.Fd())) },
		EnableVT:   func() bool { return enableVT(os.Stdout) },
		Size:       terminalWidth,
		Resize:     resizeEvents,
		After:      time.After,
		NewLoader:  usage.NewLoader,
		Refresh:    claudecli.NewRefresher().Refresh,
		Version:    version,
	}
}

func terminalWidth() (int, error) {
	for _, f := range []*os.File{os.Stdout, os.Stderr, os.Stdin} {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w, nil
		}
	}
	return 0, errors.New("not a terminal")
}

// Main runs the CLI and returns the exit code: 0 ok, 1 error, 2 weekly ahead of pace.
func Main(d Deps) int {
	fail := func(err error) int {
		fmt.Fprintf(d.Stderr, "%s: %v\n", Program, err)
		return 1
	}
	opts, err := ParseArgs(d.Args, 60)
	if err != nil {
		return fail(err)
	}
	if opts.Help {
		io.WriteString(d.Stdout, Help)
		return 0
	}
	if opts.Version {
		fmt.Fprintf(d.Stdout, "%s %s\n", Program, d.Version)
		return 0
	}
	now, err := NowFunc(d.Getenv)
	if err != nil {
		return fail(err)
	}
	loader, err := d.NewLoader(opts.From, d.Stdin, time.Duration(opts.TTL)*time.Second, now)
	if err != nil {
		return fail(err)
	}
	// Watch mode stops calling the endpoint while an expired token has not changed;
	// --refresh-token also asks Claude Code to refresh it.
	refresh := opts.RefreshToken || d.Getenv(RefreshEnv) == "1"
	if loader.Source != nil && (refresh || opts.Watch) {
		var f func(context.Context) error
		if refresh {
			f = d.Refresh
		}
		loader.Source = usage.NewRecoveringSource(loader.Source, f, now)
	}

	if opts.Raw {
		res, err := loader.Load(d.Ctx, false)
		if err != nil {
			return fail(err)
		}
		var out bytes.Buffer
		if err := json.Indent(&out, bytes.TrimSpace(res.Data), "", "  "); err != nil {
			return fail(usage.ErrInvalidJSON)
		}
		out.WriteByte('\n')
		d.Stdout.Write(out.Bytes())
		return 0
	}

	vt := d.IsTerminal() && d.EnableVT()
	color := opts.Color == "always" ||
		(opts.Color == "auto" && vt && d.Getenv("NO_COLOR") == "" && d.Getenv("TERM") != "dumb")
	st := NewStyle(color, opts.ASCII)

	if opts.Watch {
		loader.Backoff = true
		return watch(d, opts, loader, now, st)
	}

	res, err := loader.Load(d.Ctx, false)
	if err != nil {
		code := fail(err)
		var le *usage.LoadError
		if errors.As(err, &le) && strings.HasPrefix(le.Msg, "HTTP 401") && !refresh {
			fmt.Fprintf(d.Stderr, "%s: tip: --refresh-token lets Claude Code refresh an expired token\n", Program)
		}
		return code
	}
	out, rows, err := frame(d, opts, res, now(), st)
	if err != nil {
		return fail(err)
	}
	io.WriteString(d.Stdout, out)
	if pace.WeeklyAhead(rows) {
		return 2
	}
	return 0
}

// frame parses the data and renders one screen at the current terminal width.
func frame(d Deps, opts Options, res usage.Result, at time.Time, st Style) (string, []pace.Row, error) {
	snap, err := usage.Parse(res.Data)
	if err != nil {
		return "", nil, err
	}
	rows := pace.Rows(snap.Windows, at.Unix(), opts.Band)
	cols := columns(d, opts)
	f := Frame{
		Program: Program, Cols: cols, Compact: opts.Compact || cols < 60,
		Now: at.In(d.Loc), Result: res, Rows: rows, Extra: pace.ExtraFooter(snap.Extra),
	}
	return Render(f, st), rows, nil
}

// columns: --width, else the terminal, else $COLUMNS, else 80.
func columns(d Deps, opts Options) int {
	if opts.Width >= 0 {
		return opts.Width
	}
	if w, err := d.Size(); err == nil && w > 0 {
		return w
	}
	if w, err := strconv.Atoi(strings.TrimSpace(d.Getenv("COLUMNS"))); err == nil && w > 0 {
		return w
	}
	return 80
}

// NowFunc returns a clock fixed at $PACENOTCH_NOW, or the real clock.
func NowFunc(getenv func(string) string) (func() time.Time, error) {
	v := getenv(NowEnv)
	if v == "" {
		return time.Now, nil
	}
	sec, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("%s must be Unix seconds, not %q", NowEnv, v)
	}
	t := time.Unix(sec, 0)
	return func() time.Time { return t }, nil
}
