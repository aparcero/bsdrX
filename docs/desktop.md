# Desktop development

The desktop app follows the Wails v3 structure and tool versions in the
`wails-starter` reference. It preserves every existing control in `src/webui.c`,
which remains the single source for the desktop and standalone panels.

## Local workflow

From the repository root, run `mise trust`, `mise install`, and `just run`.
Use `mise exec -- just run` if mise is not activated in the shell. `just build`
builds both `build/bsdr_agent` and `build/bsdrx-desktop`. You can also launch
`build/bsdrx-desktop` directly from any working directory; it finds the agent
beside its own executable.

`just agent-build` and `just agent-run` retain the existing C-only workflow.
`just run --control-only` opens the desktop controls without streaming. Media
and pairing flags pass through to the C child. The native wrapper owns the
web listener and browser flags, and explains conflicting flags on stderr.

The app and dependencies are not installed system-wide. The C agent still uses
the available host media libraries. Linux Wails builds additionally need GTK 4
and WebKitGTK 6.0 headers/libraries. Use `just doctor` to inspect prerequisites.

## Architecture and state

- `desktop/main.go` creates the native window and restores its geometry.
- `desktop/internal/agent` launches, checks, and stops the owned C process.
- `desktop/internal/panel` forwards the original page and API through Wails'
  asset handler, converting only trusted native origins to the private
  loopback origin accepted by the C API. Foreign origins are rejected.
- `desktop/internal/windowstate` is adapted from the starter, including its
  tests. Its first-maximised-window fallback handles tiling window managers.
- `desktop/Taskfile.yml` is the Wails build layer invoked by the root justfile.

The child listens for controls on an automatically selected loopback port.
The original LAN discovery and headset pairing ports are unchanged. It runs
with `--no-browser`, so only one application window opens. A failed startup or
unexpected child exit shows an error/retry page with the local log location.
Closing the window quits this desktop app and stops its child; this version
does not minimize to a tray.

The default data directory is `build-local/` next to the repository's `build/`
directory. Config, model cache, desktop geometry, and logs use its `config/`,
`cache/`, `desktop/`, and `logs/` subdirectories. On Linux the WebView's profile
and disk cache also stay under `data/` and `cache/`. `BSDRX_DATA_DIR` can select a
different directory for an isolated test run. `BSDRX_AGENT` can select a
different agent executable; its sibling `plugins/` directory is used.

## Verification

`just test` runs the existing C suites and Go tests. Go tests cover child
startup, failed/missing executables, readiness timeouts, retry after a crash,
reaping on shutdown, API forwarding, origin checks, and window persistence.
`just test-desktop` runs the Go tests alone. `just check-desktop` checks Go
formatting and static analysis; `just test-desktop-race` runs the race detector.

For native inspection, run `just dev-agent --control-only`. It builds a
separate `build/bsdrx-desktop-dev` executable with the Wails `mcp` tag and
exposes the application's structured inspection endpoint at
`http://127.0.0.1:9099/mcp`. `WAILS_MCP_PORT` can select a different port. This is
an explicit development mode: `just build` and `just run` do not enable MCP.
The native controls are available to `app_info`, `js_eval`, `dom_query`, and
`window_control` through that endpoint. Stop the app after inspection and
confirm the child agent is gone.

Linux native validation exercises the real WebKit window. Headset pairing,
screen/audio streaming, native permission prompts, and Windows/macOS behavior
need testing on their respective hardware. On Windows the current subprocess
fallback terminates the child directly; graceful signal shutdown is implemented
on Unix. Existing release installers/bundles and the Android app continue to
use the standalone C application; Wails release packaging is not added here.
