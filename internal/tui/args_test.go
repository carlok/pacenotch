package tui

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseArgs(t *testing.T) {
	def := Options{Interval: 60, Band: 5, TTL: 60, Width: -1, Color: "auto"}
	with := func(f func(*Options)) Options { o := def; f(&o); return o }
	tests := []struct {
		args []string
		want Options
	}{
		{nil, def},
		{[]string{"-c"}, with(func(o *Options) { o.Compact = true })},
		{[]string{"--compact"}, with(func(o *Options) { o.Compact = true })},
		{[]string{"-w"}, with(func(o *Options) { o.Watch = true })},
		{[]string{"-w", "5"}, with(func(o *Options) { o.Watch, o.Interval = true, 5 })},
		{[]string{"-w", "0"}, with(func(o *Options) { o.Watch, o.Interval = true, 1 })},
		{[]string{"--watch=7"}, with(func(o *Options) { o.Watch, o.Interval = true, 7 })},
		{[]string{"-w", "-c"}, with(func(o *Options) { o.Watch, o.Compact = true, true })},
		{[]string{"-b", "3"}, with(func(o *Options) { o.Band = 3 })},
		{[]string{"--band=2.5"}, with(func(o *Options) { o.Band = 2.5 })},
		{[]string{"--ttl", "180"}, with(func(o *Options) { o.TTL = 180 })},
		{[]string{"--width", "0"}, with(func(o *Options) { o.Width = 0 })},
		{[]string{"--width=90"}, with(func(o *Options) { o.Width = 90 })},
		{[]string{"--from", "-"}, with(func(o *Options) { o.From = "-" })},
		{[]string{"--from=a.json"}, with(func(o *Options) { o.From = "a.json" })},
		{[]string{"--raw"}, with(func(o *Options) { o.Raw = true })},
		{[]string{"--color", "always"}, with(func(o *Options) { o.Color = "always" })},
		{[]string{"--color=never"}, with(func(o *Options) { o.Color = "never" })},
		{[]string{"--color"}, def},
		{[]string{"--no-color"}, with(func(o *Options) { o.Color = "never" })},
		{[]string{"--ascii"}, with(func(o *Options) { o.ASCII = true })},
		{[]string{"--version"}, with(func(o *Options) { o.Version = true })},
		{[]string{"--refresh-token"}, with(func(o *Options) { o.RefreshToken = true })},
		{[]string{"-h", "--bogus"}, with(func(o *Options) { o.Help = true })},
		{[]string{"--help"}, with(func(o *Options) { o.Help = true })},
	}
	for _, tt := range tests {
		got, err := ParseArgs(tt.args, 60)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseArgs(%q) = %+v, %v\nwant %+v", tt.args, got, err, tt.want)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--nope"}, "unknown option: --nope (try --help)"},
		{[]string{"x"}, "unknown option: x (try --help)"},
		{[]string{"--raw=1"}, "unknown option: --raw=1 (try --help)"},
		{[]string{"-b"}, "--band needs a number"},
		{[]string{"-b", "-1"}, "--band needs a number"},
		{[]string{"-b", "1."}, "--band needs a number"},
		{[]string{"--ttl", "x"}, "--ttl needs seconds"},
		{[]string{"--ttl", "99999999999999999999"}, "--ttl needs seconds"},
		{[]string{"--width"}, "--width needs a number"},
		{[]string{"--watch=x"}, "--watch needs seconds"},
		{[]string{"--from"}, "--from needs a file"},
		{[]string{"--from="}, "--from needs a file"},
		{[]string{"--color", "sometimes"}, `--color must be auto, always or never, not "sometimes"`},
	}
	for _, tt := range tests {
		if _, err := ParseArgs(tt.args, 60); err == nil || err.Error() != tt.want {
			t.Errorf("ParseArgs(%q) error = %v, want %q", tt.args, err, tt.want)
		}
	}
}

func TestHelpExplainsTheNotch(t *testing.T) {
	if !strings.Contains(Help, "The notch ┃ marks an even pace") || !strings.Contains(Help, "not affiliated with Anthropic") {
		t.Error("help must explain the notch and the unofficial status")
	}
}
