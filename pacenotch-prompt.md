# Task: `pacenotch`, a cross-platform Claude usage monitor with a vertical pace notch (CLI + GUI) for macOS, Windows and Ubuntu

## Context

`reference/claude-pace.sh` is a working bash script that shows my Claude plan usage limits
(5-hour and weekly windows) compared with a linear pace. It is the **reference
implementation**: the new tool must reproduce its behaviour, numbers and terminal output.
Read it completely before writing any code. It only runs where bash, curl and jq exist; I
want native binaries for macOS, Windows and Ubuntu, with a GUI as well as the terminal view.

**Name.** The new project is called **`pacenotch`**. The name `claude-pace` is already taken,
and a name without "Claude" also avoids implying an official Anthropic product. Use
`pacenotch` for the repo, the Go module, the binaries, the cache directory and the
environment variables. Only the reference script keeps its old file name.

**The vertical pace notch is the whole point of this project.** Every bar shows a thin
**vertical** notch at the position where usage would be if it were spent evenly over the
window (the elapsed fraction of the window). Fill past the notch means ahead of pace; fill
short of it means room to spare. Existing usage monitors show only "percent used", which
cannot tell you whether 60% on a Thursday is fine or a problem. The notch answers that at a
glance, and it is the reason this project exists. Hold every design decision to these rules:

- **Always present.** Every active window's bar has the notch, in every renderer: terminal,
  GUI window and tray icon. Only idle windows, which have no elapsed time, have none.
- **Always vertical.** Bars are always horizontal and the notch always crosses them
  vertically, including in the tray icon. Never switch to vertical bars with a horizontal
  tick.
- **Always distinct.** The notch must stand out from both the fill and the empty part, in
  light and dark themes, with color on or off, and in `--ascii` mode.
- **Always moving.** The notch moves with time even when no new data arrives, so every
  renderer redraws on a timer.
- **Explained up front.** The README opens with an annotated example of a bar that points
  at the notch, and `-h` explains it in one line.

## Deliverables

- **CLI-only build.** Pure Go, `CGO_ENABLED=0`, cross-compiled from any machine for
  darwin/arm64, darwin/amd64, linux/amd64 and windows/amd64. This is the fallback if GUI
  builds cause trouble, so it must stand on its own.
- **GUI build.**
  - macOS: one binary where `pacenotch` is the CLI and `pacenotch gui` opens the GUI, plus
    a `.app` bundle with no Dock icon (`LSUIElement`).
  - Ubuntu 22.04+: one binary, same `gui` subcommand.
  - Windows: **two executables**, because one exe cannot cleanly be both a console app and a
    GUI app. They are `pacenotch.exe` (console subsystem, CLI) and `pacenotch-gui.exe`
    (`-ldflags -H=windowsgui`).
- **GitHub Actions workflow.** A matrix on macos-latest, windows-latest and ubuntu-latest
  that runs the tests with `-race`, the fuzz tests and the coverage gates, uploads coverage
  reports, builds the GUI binaries natively and attaches all artifacts to a release on tag
  push.
- **README.** It opens with the tagline "Claude usage limits with a vertical pace notch" and
  an annotated bar example that points at the notch and explains it. It states that the
  project is unofficial and not affiliated with Anthropic. Then come install steps, flags, and
  platform notes:
  - macOS: unsigned-binary Gatekeeper workaround, `xattr -d com.apple.quarantine`.
  - Ubuntu: the tray needs the AppIndicator extension, which Ubuntu enables by default.
  - Windows: the `--ascii` fallback.

## Architecture

- `internal/usage`: credentials, HTTP fetch, cache and parsing of both input formats. No UI code.
- `internal/pace`: pure functions from (windows, now, band) to display rows. Table-driven tests.
- `internal/tui`: terminal renderer and watch mode.
- `internal/gui`: Fyne window and tray, behind a build tag so the CLI-only build excludes it.
- **One shared bar rasterizer**, `drawBar(img, rect, row, theme)`, horizontal bars only. It draws both
  the GUI window bars (inside a `canvas.Raster`, so the bars stretch with the window) and the
  tray icon.
- `now` must be injectable. Use the `PACENOTCH_NOW` environment variable (Unix seconds) for
  tests.

## Data source (same as the reference)

**Endpoint.** `GET https://api.anthropic.com/api/oauth/usage` with the headers
`Authorization: Bearer <token>` and `anthropic-beta: oauth-2025-04-20`. It is undocumented,
so treat every field as optional and never crash on unexpected shapes. Save this sample in
`testdata/`:

```json
{"five_hour":{"utilization":33.0,"resets_at":"2026-09-11T14:00:00.528743+00:00"},
 "seven_day":{"utilization":72.0,"resets_at":"2026-09-14T00:59:59.951713+00:00"},
 "seven_day_opus":null,
 "seven_day_sonnet":{"utilization":0.0,"resets_at":null},
 "extra_usage":{"is_enabled":false,"monthly_limit":null,"used_credits":null,"utilization":null}}
```

**Status line format.** Also accept the Claude Code status line format, both wrapped
(`{"rate_limits":{"five_hour":{"used_percentage":12,"resets_at":1789347599},...}}`) and
unwrapped. `resets_at` can be ISO 8601 with fractional seconds and any UTC offset, Unix epoch
seconds, or null.

**Token.**
- Lookup order: the `PACENOTCH_TOKEN` environment variable first, then the OS store.
- macOS: run `security find-generic-password -s "Claude Code-credentials" -w`, which returns
  JSON. If that fails, fall back to the file below.
- Linux: `~/.claude/.credentials.json`.
- Windows: `%USERPROFILE%\.claude\.credentials.json`.
- Fields: read `claudeAiOauth.accessToken`, and warn if `claudeAiOauth.expiresAt`
  (milliseconds) is in the past.
- **Read-only:** never refresh, write or log the token. It must not appear in any output or
  error message.

**Cache.**
- Location: `os.UserCacheDir()/pacenotch/usage.json`, with `--ttl` seconds.
- Failed fetch: show the cached data marked STALE, with the age and reason.
- 401: message "start Claude Code once to refresh the token".
- 429: back off, from 5 up to 30 minutes, in watch and GUI modes.

**Endpoint discipline.** The endpoint rate-limits aggressively. Tests use fixtures and
`httptest` only. Call the real endpoint at most a couple of times in total, with
`pacenotch --raw`, to confirm it works.

## Pace math (must match the reference exactly)

**Selection and order.**
- Keep keys whose value is an object with a numeric `utilization` (or `used_percentage`).
- Keys that are `null`, such as `seven_day_opus: null`, are hidden. Utilization 0 is shown.
- Window length: keys starting with `five_hour` are 18000 s, keys starting with `seven_day`
  are 604800 s. Skip all other keys.
- Order: `five_hour`, then `seven_day`, then the rest.

**Idle rows.** If `resets_at` is null, the row is idle with `used = utilization`. If the reset
time is at or before `now`, the row is idle with `used = 0`. Idle rows have no notch: they
show an empty bar, "not started", and "starts with your next message".

**Active rows.**
- `left = reset - now`
- `elapsed = clamp(W - left, 0, W)`
- `expected = elapsed / W * 100`
- `delta = used - expected`
- Status: `ahead` if `delta > band`; `on` if `delta >= -band`; otherwise `under`.
  `--band` defaults to 5.
- `unit` is a day for 7d windows and an hour for 5h windows.
- `budget = (100 - used) / (left / unit)`
- `flat = 100 / (W / unit)`, which gives 14.3/day and 20.0/h.

**Projection.**
- Compute it only when `elapsed >= 0.05 * W` and `used > 0`: `proj = used / (elapsed / W)`.
- `hit` (seconds until the limit):
  - `used >= 100`: `hit = 0`.
  - else `proj > 100`: `hit = (100 - used) / (used / elapsed)`.
- Output:
  - `hit < left`: "limit in X, Y before reset", in red if the row is ahead, otherwise yellow.
  - otherwise: "~proj% at reset".
  - no projection computed: "too early in the window to project".

**Labels.**
- Full: "5h session", "7d all models", "7d Sonnet", "7d Opus"; any other `seven_day_X` is "7d X".
- Compact: "5h", "7d", "7d S", "7d O".
- If `extra_usage.is_enabled` is true, add the footer "extra usage: on (N% of monthly limit)".

## Terminal output

- **Parity.** With `--color never --width N`, the output must be identical to the reference
  for the same input and the same `now`. To test this, copy the reference to
  `testdata/reference.sh` and patch only that copy: it reads `now` from `PACENOTCH_NOW`, and
  its header prints `pacenotch` instead of `claude-pace`. The program name is the only
  difference the patch may introduce.
  Then write golden tests that run both on every fixture at widths 40, 59, 60, 90 and 200.
  Skip these tests where bash or jq is missing.
- **Layout.**
  - Views: the full view, and the compact view (`-c`, switched on automatically below 60
    columns).
  - Every line leaves a one-column right margin.
  - Bars fill to 1/8-cell precision using the eighth-block characters.
  - Terminal width comes from `golang.org/x/term`, and `--width` overrides it.
- **Colors.**
  - Status: red for ahead, yellow for on pace, green for under, cyan for idle.
  - Empty cells: gray.
  - Notch: bright white. A notch inside the filled part also gets the status color as its
    background.
  - Color modes: `--color auto|always|never` and `--no-color`; honor `NO_COLOR`.
- **Windows console.** Enable virtual-terminal processing, and add `--ascii` (`#`, `-`, `|`)
  for consoles that cannot render block characters.
- **Flags and exit codes.** Keep the reference flags: `-c`, `-w [secs]`, `-b`, `--ttl`,
  `--width`, `--from FILE|-`, `--raw`, `--color`, `--no-color`, `-h`, plus `--ascii`. In
  single-shot mode, exit with 0 when fine, 1 on error, and 2 when "7d all models" is ahead
  of pace.
- **Watch mode.**
  - Build each frame fully, then clear the screen and write it in one go, so there is no
    flicker.
  - Hide the cursor, and restore it on Ctrl-C.
  - Redraw immediately on resize: SIGWINCH on Unix, polling the size on Windows.
  - Redraw at every interval even without new data, because the notch moves with time.
    Fetch only when the TTL expires.

## GUI

- **Launching.** `pacenotch gui`, or `pacenotch-gui.exe` on Windows. It accepts `-b`,
  `--ttl` (default 180 in the GUI) and `--from`.
- **Tray / menu bar icon.** Rendered dynamically:
  - Two **horizontal** bars stacked: 5h on top, 7d below. Each fills left to right to the
    used percentage and carries the **vertical pace notch**, exactly like the terminal and
    window bars.
  - Shape: on macOS, use a wide image (about 2:1, scaled to the menu bar height), since the
    menu bar allows it. On Windows and Linux, tray icons are square, so fit both bars into
    the square.
  - Size: legible at 16, 22 and 32 px. At 16 px the notch must still be at least 1 px wide in
    a contrasting color, with a 1 px gap on each side if needed to stand out from the fill.
  - Colored by status, gray or hollow when idle, and a small warning mark when the data is
    stale or authentication fails.
- **Tray menu.**
  - One text line per window, e.g. `7d  72% / pace 64%  ▲ slow down  · resets 2d 13h`.
  - Then "Open window", "Refresh now" and "Quit".
- **Window.** The full view, rendered natively:
  - A header with the data age, or STALE and the reason.
  - For each window: the label and status line, then a horizontal bar with the **vertical
    notch** that stretches with the window width, then the budget and projection lines.
  - Redraw every 60 s, and fetch according to the TTL.
  - Follow the OS light/dark theme.
  - Closing the window leaves the tray running; Quit exits.
- **Notification (optional).** A native notification once each time "7d all models" changes
  into the ahead state.

## Technology

- Use a recent stable Go. The CLI should need little beyond the standard library,
  `golang.org/x/term` and `golang.org/x/sys` (for the Windows console mode).
- Build the GUI with Fyne v2: the tray through `desktop.App`, and the bars through
  `canvas.Raster`.
- If Fyne blocks something important (tray icons, packaging, rendering), stop and tell me
  which alternative you propose, for example Wails or Rust eframe/egui with `tray-icon`.
  Don't switch silently.

## Unit tests and code coverage

Testing is part of every phase, not a final step. No phase is done until its tests pass and
its packages meet their coverage thresholds.

**Design for testability.**
- Put every side effect behind a small interface, with a fake in tests: the clock, the
  credential source (including the macOS `security` call through an injectable command
  runner), the HTTP client, the filesystem (cache directory from `t.TempDir()`) and the
  terminal size.
- Keep Fyne glue thin. Everything the GUI shows (row text, colors, icon pixels, stale and
  error states) comes from a view-model and the shared rasterizer, which are plain Go and
  fully unit-testable.

**Unit tests, per package.**
- `internal/pace`: table-driven tests for every branch of the pace math.
  - Boundaries: `delta` exactly at `+band` and `-band`, 0% and 100% used, used above 100.
  - Times: `elapsed` exactly at 5% of the window, a reset exactly at `now` and one second
    before or after, `left` larger than the window.
  - Rounding and formatting: one-decimal output (`14.3`, `20.0`), durations below one
    minute, one hour and one day.
- `internal/usage`:
  - Parsing of both input formats. ISO values with and without fractional seconds, with
    `Z`, `+02:00` and `-05:30`; epoch values; null values.
  - Unknown extra keys, wrong types and truncated JSON.
  - Each credential source and its fallback order, and expired-token detection.
  - HTTP 200, 401, 429, 500, timeout and invalid body, using `httptest`.
  - Cache hit, miss and expiry, and falling back to stale data on error.
  - 429 backoff timing, using the fake clock.
- `internal/tui`:
  - Bar rendering at each eighth-block remainder, with the notch at the first cell, the last
    cell, inside the fill and outside it, and with no notch (idle).
  - Width fitting, compact switching at 59/60 columns, and the one-column margin, checked by
    counting runes, not bytes.
  - Color on, color off and `--ascii`.
  - Exit codes 0, 1 and 2.
- `internal/gui`:
  - View-model output for every row state (ahead, on, under, idle, stale, auth error).
  - Rasterizer tests that check pixel colors at the notch position, the fill edge and the
    empty area, at 16, 22, 32 and 44 px. They must also verify that the notch is vertical,
    at least 1 px wide, and contrasts with both the fill and the empty part, in light and dark
    themes.
  - Use Fyne's `test` driver for basic widget tests: the window builds, the tray menu entries
    exist, and Refresh calls the fetcher.

**Golden files.** Store expected terminal output in `testdata/golden/`. A `-update` test flag
regenerates them. Review every golden diff as part of the change that caused it.

**Security test.** Run every code path (success, each error, `--raw`, the stale banner and
the cache writes) with a recognizable fake token. Assert that the token string never appears
in stdout, stderr, returned errors, logs or cache files.

**Fuzzing.** Add Go native fuzz tests for the timestamp parser and the response parser. The
property is "never panics, and returns either valid rows or an error". Run each for 30 s in
CI (`-fuzztime=30s`), and commit any crashers found as regression cases in `testdata/fuzz/`.

**Race detector.** Run every CI test run with `-race`, because watch mode and the GUI share
state between the refresh timer, the fetcher and the renderer.

**Coverage.**
- Unit coverage: `go test -race -covermode=atomic -coverprofile=coverage.out ./...`.
- End-to-end coverage: build the CLI with `go build -cover` and run the parity and exit-code
  tests against that binary with `GOCOVERDIR` set. Merge with `go tool covdata` into the
  unit profile, so `main` and flag handling are measured too.
- Minimum statement coverage, enforced in CI by a small script that reads
  `go tool cover -func` and fails the job when a package is below its threshold:

  | Package | Minimum |
  |---|---|
  | `internal/pace` | 100% |
  | `internal/usage` | 90% |
  | `internal/tui` | 90% |
  | `internal/gui`, view-model and rasterizer | 90% |
  | `internal/gui`, Fyne glue | no gate (keep it thin) |
  | whole module, merged unit + end-to-end | 85% |

- Measure OS-specific files (the Windows console mode, credential paths, the macOS
  `security` call) in that OS's CI job, which applies the thresholds to what that OS
  compiles.
- Don't lower a threshold or exclude code to pass the gate. If something truly can't be
  tested, explain why in a comment and in your report to me.

**Make targets.**
- `make test`: race-enabled unit tests.
- `make cover`: the merged profile and the per-package summary.
- `make cover-html`: `coverage.html`, which CI also uploads as an artifact from each OS job.
- `make fuzz`: short fuzz runs.
- `make golden`: regenerate the golden files.

**Reporting.** At each checkpoint, include the `go tool cover -func` summary per package, and
list which uncovered lines remain and why.

## Work plan: stop at each checkpoint, report, and wait for my OK

1. Read the reference. Set up the Makefile, the coverage-gate script and a first CI job,
   so tests and coverage run from the first commit.
2. Implement `internal/usage` and `internal/pace` with their unit tests, fuzz tests and the
   token-leak test. The fixtures must cover:
   - windows that are idle, already reset, `null`, and at 0%
   - the status line format, with epoch and ISO values and different offsets
   - 401, 429 and malformed JSON, using `httptest`

   `internal/pace` must be at 100% coverage and `internal/usage` at 90% or more.
3. Build the CLI renderer, the golden files, the parity tests against the reference, the
   end-to-end coverage run, and the CLI-only cross-compiled binaries. **Checkpoint:** I run
   them on my three machines, and you send the coverage report.
4. Build the GUI (window and tray) on my current OS, with view-model and rasterizer tests.
   **Checkpoint**, with the coverage report.
5. Complete the CI matrix: tests, `-race`, fuzzing and coverage gates on all three OSes, and
   coverage HTML artifacts. Add the packaging (the `.app` bundle and the Windows GUI exe),
   and write the README, including how to run the tests and read the coverage.

## Acceptance criteria

- `go test -race ./...` passes on all three operating systems in CI. The parity tests pass
  wherever bash and jq are available.
- The coverage gates pass on all three operating systems with the thresholds from the
  coverage section, and each OS job uploads a coverage HTML report.
- The fuzz tests run in CI without crashes, and every past crasher is kept as a regression
  case.
- On macOS, Ubuntu and Windows, the CLI's terminal output matches the reference at several
  widths, and window resizing works in watch mode.
- The vertical pace notch appears on every active bar in every renderer (terminal, window,
  tray icon), at the correct position, and is clearly visible in both themes and in
  `--ascii` mode.
- The GUI tray icon and window both update without new data as time
  passes, and survive endpoint errors by showing stale data instead of crashing.
- The token never appears in any output, log, cache file or error message.
- The real endpoint is called a few times at most during the whole development session.
