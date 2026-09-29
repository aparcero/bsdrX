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

# Build the agent, plugins, tools, and tests; configure on the first build.
build:
    @./scripts/dev.sh build

# Build and run the repository test suites with separate local settings.
test: build
    @./scripts/dev.sh test

# Build and launch with settings and caches inside this checkout.
run *args: build
    @./scripts/dev.sh run "$@"

# Remove compiled application outputs; retain dependencies and local settings.
clean:
    @make clean
