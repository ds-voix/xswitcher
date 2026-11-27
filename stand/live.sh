#!/bin/bash
# Long-lived stand container.
#
# Why this exists: `docker run --rm` pays container creation on every invocation (measured 4.7s for
# `docker run --privileged xswitcher-stand true`), and a fresh container starts with an empty GOCACHE,
# so every edit-rebuild cycle recompiled the Go standard library before reaching the phases. The
# container created here stays up, so `run` only costs the reset and the phases themselves.
#
# Subcommands:
#   bash stand/live.sh up      # create/start the container (idempotent), loads host modules first
#   bash stand/live.sh reset   # drop everything a previous run left behind
#   bash stand/live.sh run     # reset, then the whole stand (streamed, exit code of run.sh)
#   bash stand/live.sh sh      # a shell inside it
#   bash stand/live.sh logs    # the daemon log of the last phase 12 (X11 branch)
#   bash stand/live.sh down    # remove the container
#
# `run.sh` is not written to be re-entrant: it starts Xvfb on :99, creates a source keyboard through
# uinput, and mknod's device nodes. Left alone, the second run in one container inherits a live Xvfb
# with the keymap phase 12 left on it, a second stand-source-keyboard, and nodes whose kernel device
# is already gone - which is how the first experiment here failed in `UInput()`. So `reset` kills the
# leftovers, removes nodes no longer backed by /sys/class/input, frees the X99 socket and lock, and
# drops the /tmp artifacts (the binaries are rebuilt by run.sh itself).
#
# The image has no procps: pkill, pgrep and ps are all MISSING (measured with `command -v` inside
# xswitcher-stand), so the killer walks /proc and matches cmdlines. The reset script's own shell
# would match its patterns, hence the explicit skip of $$.
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/.." && pwd)
DOCKER="${DOCKER:-/f/Docker/resources/bin/docker}"
NAME="${NAME:-xswitcher-live}"
IMAGE="${IMAGE:-xswitcher-stand}"
WS=$(cygpath -w "$REPO")

d() { MSYS_NO_PATHCONV=1 "$DOCKER" "$@"; }

say() { printf '%-46s %s\n' "$1" "${2:-}"; }

running() { d inspect -f '{{.State.Running}}' "$NAME" 2>/dev/null | grep -q '^true$'; }

RESET='
self=$$
for sig in TERM KILL; do
    for p in /proc/[0-9]*; do
        pid=${p#/proc/}
        [ "$pid" = "$self" ] && continue
        cmd=$(tr "\0" " " < "$p/cmdline" 2>/dev/null)
        case "$cmd" in
            *xswitcher*|*Xvfb*|*source_key.py*|*sniff.py*|*/tmp/mkwin*|*/tmp/xgroup*)
                kill -"$sig" "$pid" 2>/dev/null || true ;;
        esac
    done
    [ "$sig" = TERM ] && sleep 0.5
done
for n in /dev/input/event*; do
    [ -e "$n" ] || continue
    [ -e "/sys/class/input/${n##*/}" ] || rm -f "$n"
done
rm -f /tmp/.X99-lock /tmp/.X11-unix/X99
rm -f /tmp/xswitcher /tmp/mkwin /tmp/xgroup /tmp/xxx
rm -f /tmp/*.conf /tmp/*.log /tmp/*.tsv
true
'

case "${1:-up}" in
    up)
        [ -d "$REPO/.gomodcache" ] || mkdir -p "$REPO/.gomodcache"
        d images -q "$IMAGE" | grep -q . || { echo "FATAL: image $IMAGE is not built (see stand/Dockerfile)"; exit 9; }
        if running; then
            say "container $NAME" "already running"
        else
            [ -e /dev/uinput ] || bash "$HERE/setup-host.sh" || { echo "host modules unavailable"; exit 9; }
            d rm -f "$NAME" >/dev/null 2>&1
            d run -d --name "$NAME" --privileged \
                -e DUMPLOG="${DUMPLOG:-}" \
                -v "$WS:/w" \
                -v "$WS/.gomodcache:/root/go/pkg/mod" \
                "$IMAGE" sleep infinity >/dev/null
            for i in $(seq 20); do running && break; sleep 0.3; done
            running || { echo "FATAL: container $NAME did not come up"; exit 9; }
            echo "container $NAME started"
        fi
        ;;
    reset)
        running || { echo "FATAL: no container $NAME, run: bash stand/live.sh up"; exit 9; }
        d exec "$NAME" bash -lc "$RESET"
        echo "stand reset"
        ;;
    run)
        running || { echo "FATAL: no container $NAME, run: bash stand/live.sh up"; exit 9; }
        d exec "$NAME" bash -lc "$RESET"
        d exec "$NAME" bash /w/stand/run.sh
        ;;
    sh)
        d exec -it "$NAME" bash
        ;;
    logs)
        d exec "$NAME" tail -60 /tmp/x11.log
        ;;
    down)
        d rm -f "$NAME" >/dev/null && echo "container $NAME removed"
        ;;
    *)
        echo "usage: bash stand/live.sh [up|reset|run|sh|logs|down]"; exit 2
        ;;
esac
