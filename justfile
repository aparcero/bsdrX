set positional-arguments

# List the available development tasks.
default:
    @just --list

# Prepare the pinned SCTP library under build-local/deps.
deps:
    @./scripts/dev.sh deps

# Check host libraries and configure a local build (accepts ./configure flags).
configure *args:
    @./scripts/dev.sh configure "$@"

# Build the C agent, plugins, tools, and tests without the desktop window.
agent-build:
    @./scripts/dev.sh build

# Build the native Wails desktop app and its C streaming agent.
build: agent-build
    @cd desktop && wails3 build

# Run C suites and Go desktop lifecycle/proxy/window-state tests.
test: agent-build
    @./scripts/dev.sh test
    @cd desktop && go test ./...

# Run only the desktop Go tests.
test-desktop:
    @cd desktop && go test ./...

# Check desktop formatting and Go static analysis.
check-desktop:
    @files=$(gofmt -l desktop); test -z "$files" || { printf '%s\n' "$files"; exit 1; }
    @cd desktop && go vet ./...

# Check desktop lifecycle and persistence for data races.
test-desktop-race:
    @cd desktop && go test -race ./...

# Build and launch the native desktop app with repo-local settings and caches.
run *args: build
    @./build/bsdrx-desktop "$@"

# Launch the standalone agent (including its original browser/app-window UI).
agent-run *args: agent-build
    @./scripts/dev.sh run "$@"

# Check desktop platform prerequisites.
doctor:
    @cd desktop && wails3 doctor

# Build an explicitly instrumented native app for local Wails MCP inspection.
dev-agent *args: agent-build
    @cd desktop && wails3 build DEV=true EXTRA_TAGS=mcp OUTPUT=../build/bsdrx-desktop-dev
    @WAILS_MCP_HOST=127.0.0.1 WAILS_MCP_PORT="${WAILS_MCP_PORT:-9099}" ./build/bsdrx-desktop-dev "$@"

# Remove compiled application outputs; retain dependencies and local settings.
clean:
    @make clean
