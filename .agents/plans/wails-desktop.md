# Wails desktop app

## Confirmed scope

2026-09-29: the user requested a desktop app using `~/Work/wails-starter` as a
reference/scaffold and explicitly selected keeping all current controls in a
native Wails window. Earlier constraints remain: use mise and just, run from the
checkout, and do not install the application or packages system-wide.

## Approach

Keep the C agent and its entire existing HTML/API as the source of truth. Add a
separate `desktop/` Go module using the starter's Wails v3 beta.26, native window
setup, persisted window geometry, and just → wails3 → Taskfile build flow. A Go
supervisor owns one child agent on a private loopback port, proxies the panel
through Wails' asset handler, and stops its child on shutdown. The desktop shows
an error/retry page if startup fails or the child exits. No React migration,
database, starter todo features, or starter-specific MCP configurations are
needed for the confirmed scope. The reference repository stays untouched.

`just build` and `just run` become desktop entry points; `agent-build` and
`agent-run` retain the direct C workflow. Local settings and caches remain under
`build-local/`. Existing Android and release packaging flows remain separate.

## Milestones and validation

- [x] Inspect source app, starter, runtime pins, and available native libraries.
- [x] Add supervisor, proxy/error handling, native window, and build integration.
- [x] Verify lifecycle failures, API forwarding, local paths, and shutdown with
  Go tests; retain the C suites and the starter window-state tests.
- [x] Build the production native executable and inspect the real Linux Wails
  window using development-only Wails MCP. Server-only checks are not native
  evidence. Verify settings survive restart and that shutdown reaps the child.
- [x] Update documentation with desktop/agent commands and platform limits.

## Risks and limits

The current native verification host is Linux. Windows/macOS compilation and
headset/media behavior require their own hardware testing. A desktop wrapper
must not alter the C protocol or weaken its browser-origin checks. Wails MCP is
only enabled by an explicit development build, never a production build. No
independent review has been requested or performed; the reference repository's
workflow controls are not copied into this repository.

## Outcome

Implemented and verified on Linux on 2026-09-29. `just build` produces
`build/bsdrx-desktop` with the `production` tag, without MCP. `just run` launches
the native app; `just agent-run` retains the standalone workflow. The starter
repository is unchanged.

Validation completed:

- `mise exec -- just test`: all 19 C suites and desktop Go packages passed.
- `mise exec -- just test-desktop-race`: passed.
- `mise exec -- just check-desktop`: formatting and static analysis passed.
- Native Wails MCP inspection in `--control-only` mode found 10 panels and 135
  controls, including dynamically loaded plugin controls. Updating bitrate
  through the original UI reached the C API and survived a full app restart.
- Closing the native window reaped its child, closed the development MCP
  endpoint, and saved valid window geometry. Stopping the owned child showed
  the recovery page; submitting its retry button launched a new child and
  restored the controls with the saved setting.
- WebKitGTK cache/profile directories were observed inside the isolated
  `BSDRX_DATA_DIR`, keeping those native browser artifacts in the checkout too.

Native checks exposed and resolved the custom-scheme redirect limitation on
the retry page, the need to load the Wails runtime for native events, and the
C server's requirement for explicit request body lengths. Go regression tests
cover the corresponding handler behavior. The window-state tracker also keeps
a valid fallback when a tiling window manager maximises its first observation.

Validation logs and the temporary native smoke harness are ignored artifacts
under `build-local/`. Headset streaming and Windows/macOS remain unverified;
the existing release packaging still targets the standalone C application.
