#!/usr/bin/env bash
# Local development helpers for justfile. No system package or application install.
set -euo pipefail

BSDR_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BSDR_LOCAL="$BSDR_ROOT/build-local"
BSDR_DEPS="$BSDR_LOCAL/deps"
BSDR_JOBS="${BSDR_JOBS:-$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 2)}"
case "$BSDR_JOBS" in
'' | *[!0-9]* | 0)
    echo "BSDR_JOBS must be a positive integer" >&2
    exit 1
    ;;
esac
cd "$BSDR_ROOT"

need() {
    command -v "$1" >/dev/null 2>&1 || {
        echo "Missing development tool: $1 (see README.md, Build & run)" >&2
        exit 1
    }
}

sha256_of() {
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum "$1" | cut -d' ' -f1
    else
        shasum -a 256 "$1" | cut -d' ' -f1
    fi
}

deps() {
    # Match the version used by scripts/linux-bundle.Dockerfile.
    local version=0.9.5.0
    local sha=260107caf318650a57a8caa593550e39bca6943e93f970c80d6c17e59d62cd92
    local archive="$BSDR_LOCAL/downloads/usrsctp-$version.tar.gz"
    local source="$BSDR_LOCAL/src/usrsctp-$version"
    local build="$BSDR_LOCAL/usrsctp-build"

    need pkg-config
    if [[ -f "$BSDR_DEPS/lib/libusrsctp.a" && -f "$BSDR_DEPS/include/usrsctp.h" ]] &&
        [[ "$(PKG_CONFIG_PATH="$BSDR_DEPS/lib/pkgconfig" pkg-config --modversion usrsctp 2>/dev/null || true)" == "$version" ]]; then
        echo "usrsctp $version ready in build-local/deps"
        return
    fi
    need cmake
    need make
    need curl
    need tar
    mkdir -p "$BSDR_LOCAL/downloads" "$BSDR_LOCAL/src"
    if [[ ! -f "$archive" ]]; then
        curl -fL --retry 3 --connect-timeout 15 --max-time 180 \
            "https://github.com/sctplab/usrsctp/archive/refs/tags/$version.tar.gz" \
            -o "$archive.part"
        mv "$archive.part" "$archive"
    fi
    if [[ "$(sha256_of "$archive")" != "$sha" ]]; then
        echo "SHA256 mismatch: $archive; remove this archive and retry just deps" >&2
        exit 1
    fi
    tar -xzf "$archive" -C "$BSDR_LOCAL/src"
    cmake -S "$source" -B "$build" -G "Unix Makefiles" \
        -DCMAKE_BUILD_TYPE=Release \
        -DCMAKE_INSTALL_PREFIX="$BSDR_DEPS" -DCMAKE_INSTALL_LIBDIR=lib \
        -DCMAKE_POSITION_INDEPENDENT_CODE=ON \
        -DCMAKE_POLICY_VERSION_MINIMUM=3.5 \
        -Dsctp_build_shared_lib=OFF -Dsctp_build_programs=OFF \
        -Dsctp_build_fuzzer=OFF -Dsctp_werror=OFF
    cmake --build "$build" --parallel "$BSDR_JOBS"
    # Stage only in this checkout. The application itself runs directly from build/.
    cmake --install "$build"
}

configure_local() {
    deps
    mkdir -p "$BSDR_LOCAL/tmp"
    PKG_CONFIG_PATH="$BSDR_DEPS/lib/pkgconfig${PKG_CONFIG_PATH:+:$PKG_CONFIG_PATH}" \
        CFLAGS="-I$BSDR_DEPS/include${CFLAGS:+ $CFLAGS}" \
        LDFLAGS="-L$BSDR_DEPS/lib${LDFLAGS:+ $LDFLAGS}" \
        TMPDIR="$BSDR_LOCAL/tmp" \
        ./configure --prefix="$BSDR_LOCAL/app" "$@"
}

local_runtime() {
    local state_dir=$1
    export XDG_CONFIG_HOME="$state_dir/config"
    export XDG_CACHE_HOME="$state_dir/cache"
    # BSDR_MODEL_DIR also keeps the model cache local on macOS, where the default
    # follows Library/Caches instead of XDG_CACHE_HOME.
    export BSDR_MODEL_DIR="$XDG_CACHE_HOME/bsdrX/models"
    mkdir -p "$XDG_CONFIG_HOME" "$XDG_CACHE_HOME" "$BSDR_MODEL_DIR"
}

action=${1:-}
[[ $# -eq 0 ]] || shift
case "$action" in
deps) deps ;;
configure) configure_local "$@" ;;
build)
    if [[ ! -f config.mk ]]; then
        configure_local
    else
        deps
    fi
    make -j"$BSDR_JOBS"
    ;;
test)
    local_runtime "$BSDR_LOCAL/tests"
    exec make check "$@"
    ;;
run)
    local_runtime "$BSDR_LOCAL"
    exec "$BSDR_ROOT/build/bsdr_agent" "$@"
    ;;
*)
    echo "Usage: $0 {deps|configure|build|test|run} [args...]" >&2
    exit 2
    ;;
esac
