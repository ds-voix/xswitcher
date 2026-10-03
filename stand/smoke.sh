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
    # A Phase 4 check reloads the daemon: the trap PID is the parent that
    # exited, the live process is the respawned child. The image has no
    # pkill/pgrep (procps missing), so cleanup kills the child PID recorded
    # from the reload response.
    [ -f /tmp/smoke-child.pid ] && kill "$(cat /tmp/smoke-child.pid)" 2>/dev/null
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
# The shipped default serves Wayland; on the stand X11 (Xvfb) the daemon then
# treats the XKB group as unobserved and drops every buffer before the action
# chains (checkLanguageId) - typing would never reach NewWord. The behavioral
# stand runs with its own X11 config; this file must be switched too.
sed -i 's|BypassX = true|BypassX = false|' "$CONF"
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

echo "--- 5. p4 live diary ---"
# ipcsmoke left [Diary] enabled in $CONF; the running daemon does not know
# until a reload respawns it. Then one typed word must land in the diary
# file and come back over get_diary — the only way to test the record path
# is with real key events on the source device.
DIARY=/tmp/GuiSmoke.diary.txt
rm -f "$DIARY" /tmp/smoke-child.pid
python3 - "$SOCK" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sys.argv[1])
s.sendall(b'{"cmd":"reload"}\n')
buf = b""
while not buf.endswith(b"\n"):
    d = s.recv(4096)
    if not d: sys.exit(1)
    buf += d
r = json.loads(buf)
if r.get("ok") and r.get("pid"):
    open("/tmp/smoke-child.pid", "w").write(str(r["pid"]))
sys.exit(0 if r.get("ok") else 1)
PY
check "p4 reload accepted" "$?"

UP=1
for _ in $(seq 40); do
    python3 - "$SOCK" <<'PY'
import json, socket, sys
try:
    s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    s.settimeout(2)
    s.connect(sys.argv[1])
    s.sendall(b'{"cmd":"ping"}\n')
    buf = b""
    while not buf.endswith(b"\n"):
        d = s.recv(4096)
        if not d: sys.exit(1)
        buf += d
    sys.exit(0 if json.loads(buf).get("ok") else 1)
except Exception:
    sys.exit(1)
PY
    [ $? = 0 ] && { UP=0; break; }
    sleep 0.5
done
check "p4 daemon back after respawn" "$UP"
[ "$UP" = "0" ] || { say "FATAL: daemon gone after reload" ""; tail -20 "$LOG"; exit 9; }

sleep 1   # let the respawned daemon attach the source device before typing
# The default NewWord rule 1 completes a word on the first key of the NEXT
# one (SPACE:1 ... char:1), so the typing ends with a spare letter.
python3 stand/source_key.py play "$SRC" "Q,W,E,R,T,Y,SPACE,R" >/dev/null 2>&1
check "p4 keys played" "$?"
sleep 1
grep -q qwerty "$DIARY" 2>/dev/null
check "p4 diary file recorded word" "$?"
python3 - "$SOCK" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(sys.argv[1])
s.sendall(b'{"cmd":"get_diary"}\n')
buf = b""
while not buf.endswith(b"\n"):
    d = s.recv(65536)
    if not d: sys.exit(1)
    buf += d
r = json.loads(buf)
sys.exit(0 if r.get("ok") and "qwerty" in r.get("diary", "") else 1)
PY
check "p4 get_diary serves the word" "$?"

if [ "$FAILED" = "0" ]; then echo "SMOKE_EXIT=0 ALL GREEN"; else echo "SMOKE_EXIT=1"; tail -20 "$LOG"; fi
exit "$FAILED"
