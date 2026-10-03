#!/bin/bash
# Live IPC smoke (run inside the stand container; from the host:
#   bash stand/live.sh up && bash stand/live.sh reset && MSYS_NO_PATHCONV=1 \
#   docker exec xswitcher-live bash /w/stand/smoke.sh)
#
# Brings up exactly what stand/ipcsmoke.py asserts on: an X display, an EV_KEY
# source keyboard the daemon can attach (without one the status `devices` list
# is legitimately empty -- the first smoke run failed on precisely this), the
# daemon with XSWITCHER_SOCK + -c /tmp/GuiSmoke.conf, then the checks.
#
# Fixtures are reused from run.sh by name, but the 61 behavioral phases are not
# re-run here: `bash stand/live.sh run` covers those, this script covers IPC.
set -u
cd /w

CONF=/tmp/GuiSmoke.conf
SOCK=/tmp/gui.sock
LOG=/tmp/smoke-daemon.log
DAEMON=/tmp/xswitcher-smoke
FAILED=0

say() { printf '%-46s %s\n' "$1" "${2:-}"; }
check() { # check <name> <0|1>
    if [ "$2" = "0" ]; then say "$1" "PASS"; else say "$1" "FAIL"; FAILED=1; fi
}

source_path() { # node of the stand source keyboard, whatever eventN it got
    NAME="stand-source-keyboard" python3 - <<'PY'
import os, sys, evdev
want = os.environ["NAME"]
for p in evdev.list_devices():
    try:
        d = evdev.InputDevice(p)
    except Exception as err:      # a node whose kernel device is already gone
        print("skip %s: %s" % (p, err), file=sys.stderr)
        continue
    if d.name == want:
        print(d.path); break
PY
}

cleanup() {
    [ -n "${DAEMON_PID:-}" ] && kill "$DAEMON_PID" 2>/dev/null
    [ -n "${SRC_PID:-}" ] && kill "$SRC_PID" 2>/dev/null
    [ -n "${XVFB_PID:-}" ] && kill "$XVFB_PID" 2>/dev/null
    rm -f "$SOCK"
}
trap cleanup EXIT

[ -e /dev/uinput ] || { say "FATAL: no /dev/uinput" "run stand/setup-host.sh"; exit 9; }

echo "--- 1. X display ---"
if [ ! -e /tmp/.X11-unix/X99 ]; then
    Xvfb :99 -noreset -screen 0 1024x768x16 >/tmp/smoke-xvfb.log 2>&1 &
    XVFB_PID=$!
    for _ in $(seq 20); do [ -e /tmp/.X11-unix/X99 ] && break; sleep 0.3; done
fi
export DISPLAY=:99
[ -e /tmp/.X11-unix/X99 ]; check "Xvfb :99 up" "$?"

echo "--- 2. source keyboard ---"
python3 stand/source_key.py hold >/tmp/smoke-source.log 2>&1 &
SRC_PID=$!
SRC=""
for _ in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/smoke-mknod.log 2>&1
    SRC=$(source_path)
    [ -n "$SRC" ] && break
    sleep 0.5
done
[ -n "$SRC" ]; check "source device present" "$?"
echo "source device: ${SRC:-none}"

echo "--- 3. config + build ---"
cp xswitcher.conf "$CONF"
sed -i "/^\[ScanDevices\]/,/^\[/ s|^\s*Test *=.*|Test = \"$SRC\"|" "$CONF"
echo "# precious comment" >> "$CONF"   # asserted kept by ipcsmoke after set_config
GOFLAGS=-buildvcs=false go build -o "$DAEMON" ./src/ >/tmp/smoke-build.log 2>&1
[ -x "$DAEMON" ]; check "go build ./src/" "$?"
[ -x "$DAEMON" ] || { tail -5 /tmp/smoke-build.log; exit 9; }

echo "--- 4. daemon + smoke ---"
rm -f "$SOCK"
XSWITCHER_SOCK="$SOCK" "$DAEMON" -c "$CONF" >"$LOG" 2>&1 &
DAEMON_PID=$!
for _ in $(seq 20); do [ -S "$SOCK" ] && break; sleep 0.3; done
[ -S "$SOCK" ]; check "daemon listening on $SOCK" "$?"
[ -S "$SOCK" ] || { tail -20 "$LOG"; exit 9; }

python3 stand/ipcsmoke.py "$SOCK"
SMOKE=$?
check "ipcsmoke all green" "$SMOKE"

if [ "$FAILED" = "0" ]; then echo "SMOKE_EXIT=0 ALL GREEN"; else echo "SMOKE_EXIT=1"; tail -20 "$LOG"; fi
exit "$FAILED"
