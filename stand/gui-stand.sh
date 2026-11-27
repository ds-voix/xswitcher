#!/bin/bash
# === stand/gui-stand.sh ===
# The GUI stand: a keystroke goes in as evdev, the daemon switches the layout of the live X
# server and retypes the word, and a real X client is then asked what it actually received.
#
# Why the pieces are what they are (every one of them measured, see the check list):
#   - Xvfb serves a machine with "no display hardware and no physical input devices" (Xvfb(1)),
#     so keys typed in the browser over noVNC never reach the daemon. The hand has to be a uinput
#     source device (stand/source_key.py).
#   - The daemon writes its output to its own uinput keyboard, which Xvfb equally ignores:
#     measured 0 KeyPress in the eye without stand/bridge.c and 10 with it.
#   - stand/echo.c is the eye: the delivered X keycode, its evdev equivalent, the keysym name and
#     the live Xkb group. A switch is asserted by decoded keysyms, not by a screenshot.
#   - BypassX must be false: with the shipped "true" the daemon replays Meta+digit instead of
#     XkbLockGroup, and the layout of the live server does not move at all.
#
# Run inside a --privileged container with the repository mounted at /w:
#   docker exec xswitcher-gui-stand bash /w/stand/gui-stand.sh
# Needs the host kernel modules uinput and evdev (stand/setup-host.sh) and the X packages
# (xvfb, x11-utils, imagemagick, novnc/x11vnc/websockify for the browser part) -- stand/Dockerfile.gui.
set -u
cd /w

DISPLAY_NO="${GUISTAND_DISPLAY:-96}"
SOCK="${GUISTAND_SOCK:-/tmp/guistand.sock}"
# uinput is global to the kernel of the Docker VM, so two stands running at once must not create
# keyboards with the same name -- the daemon and stand/bridge.c both resolve devices by name.
SRC_NAME="${GUISTAND_SOURCE:-gui-stand-source-keyboard}"
RUN=/tmp/guistand
CONF=$RUN.conf
LOG=$RUN-daemon.log
EYE=$RUN-eye.log
BRIDGE=$RUN-bridge.log
DAEMON_BIN=/tmp/xswitcher
GUI_BIN=/tmp/xswitcher-gui
XGROUP=/tmp/guistand-xgroup
EYE_BIN=/tmp/guistand-echo
BRIDGE_BIN=/tmp/guistand-bridge

export DISPLAY=:$DISPLAY_NO
export XSWITCHER_SOCK="$SOCK"
# The daemon's log and IPC texts are localized now; this script greps them in English.
export LC_ALL=C XSWITCHER_LANG=en GOFLAGS=-buildvcs=false

FAILED=0
TOTAL=0
# G1 8 keycode pairs + G2 1 + G3 1 + G4 2 + G5 3 + G6 1 + G8 2 + G7 2. Keep in sync with checks below.
EXPECT=20

say() { printf '%-56s %s\n' "$1" "${2:-}"; }
check() { # check <name> <0|1>
    TOTAL=$((TOTAL + 1))
    if [ "$2" = "0" ]; then say "$1" "PASS"; else say "$1" "FAIL"; FAILED=1; fi
}
# A pid file per process; kill children first, because `timeout <bin>` leaves <bin> behind.
pidfile() { echo "$2" > "$RUN-$1.pid"; }
LAST_PID=""
stop_one() {
    [ -e "$RUN-$1.pid" ] || return 0
    LAST_PID=$(cat "$RUN-$1.pid")
    if [ -n "$LAST_PID" ]; then
        pkill -P "$LAST_PID" 2>/dev/null
        kill "$LAST_PID" 2>/dev/null
    fi
    rm -f "$RUN-$1.pid"
}
stop_all() {
    for f in "$RUN"-*.pid; do
        [ -e "$f" ] || continue
        stop_one "$(basename "$f" | sed 's/.*guistand-//; s/\.pid$//')"
    done
    sleep 1
}
wait_gone() { # the killed process must really be out, or it keeps writing into our logs
    [ -n "$LAST_PID" ] || return 0
    for _ in $(seq 25); do
        [ -d "/proc/$LAST_PID" ] || return 0
        sleep 0.2
    done
    echo "  warn: pid $LAST_PID is still alive"
}
source_path() { # the eventN node of our source device, whatever number the kernel assigned
    NAME="$SRC_NAME" python3 - <<'PY'
import os, evdev
want = os.environ["NAME"]
for p in evdev.list_devices():
    try:
        d = evdev.InputDevice(p)
    except Exception:
        continue
    if d.name == want:
        print(d.path); break
PY
}
# stand/mknod-input.sh only creates nodes. When our uinput keyboards die with their processes the
# /dev/input/eventN files stay behind with no /sys/class/input entry, and opening one gives
# "No such device" -- so prune them here, before the source is created and again on exit.
prune_dead_nodes() {
    for n in /dev/input/event*; do
        [ -e "$n" ] || continue
        [ -e "/sys/class/input/$(basename "$n")" ] || rm -f "$n"
    done
}

mkdir -p "$RUN"
rm -f "$EYE" "$BRIDGE" "$LOG"
prune_dead_nodes
stop_all
trap 'prune_dead_nodes; stop_all' EXIT

echo "--- 0. tools ---"
MISSING=""
for t in Xvfb setxkbmap xwininfo import gcc python3 xdotool; do
    command -v "$t" >/dev/null || MISSING="$MISSING $t"
done
[ -z "$MISSING" ] || { echo "FATAL: not in the image:$MISSING -- install them in stand/Dockerfile.gui"; exit 9; }

echo "--- 1. X server + three layouts ---"
# Ask the server, do not trust the socket file: a container restart leaves /tmp/.X11-unix/Xnn
# behind while nobody listens on it any more.
if ! xdotool getdisplaygeometry >/dev/null 2>&1; then
    rm -f "/tmp/.X11-unix/X$DISPLAY_NO"
    # -noreset matters: Xvfb restores its initial keymap when the last client leaves, and every
    # observer here is a one-shot client, so a loaded multi-group map would vanish between calls.
    Xvfb ":$DISPLAY_NO" -noreset -screen 0 1280x1024x24 >"$RUN-xvfb.log" 2>&1 &
    pidfile xvfb $!
    for _ in $(seq 30); do xdotool getdisplaygeometry >/dev/null 2>&1 && break; sleep 0.3; done
fi
xdotool getdisplaygeometry >/dev/null 2>&1 || { echo "FATAL: Xvfb :$DISPLAY_NO did not come up"; tail -5 "$RUN-xvfb.log"; exit 9; }
setxkbmap -layout us,ru,de >/dev/null 2>&1

for src in xgroup:stand/xgroup.c:-lX11 echo:stand/echo.c:-lX11\ -lXtst bridge:stand/bridge.c:-lXtst\ -lX11; do
    name=${src%%:*}; rest=${src#*:}; file=${rest%%:*}; libs=${rest#*:}
    # shellcheck disable=SC2086
    gcc -Wall -Wextra -o "/tmp/guistand-$name" "$file" $libs >"$RUN-cc-$name.log" 2>&1 || \
        { echo "FATAL: cc $file (see $RUN-cc-$name.log)"; tail -10 "$RUN-cc-$name.log"; exit 9; }
done

echo "--- 2. G1: the evdev -> X keycode offset is a fixed +8 ---"
$XGROUP set 0 >/dev/null 2>&1
for pair in "16 24 q" "30 38 a" "44 52 z" "14 22 BackSpace" "57 65 space" "28 36 Return" "2 10 1" "24 32 o"; do
    set -- $pair
    got=$(timeout 3 "$EYE_BIN" probe "$1" 2>&1 | head -1)
    kc=$(echo "$got" | sed -n 's/.*keycode=\([0-9]*\).*/\1/p')
    nm=$(echo "$got" | sed -n 's/.*name=\([^ ]*\).*/\1/p')
    [ "$kc" = "$2" ] && [ "$nm" = "$3" ]
    check "G1 evdev $1 -> X keycode $2 ($3)" "$?"
done

echo "--- 3. source keyboard ---"
sed "s/stand-source-keyboard/$SRC_NAME/" stand/source_key.py > "$RUN-source_key.py"
python3 "$RUN-source_key.py" hold >"$RUN-source.log" 2>&1 &
pidfile source $!
SRC=""
for _ in $(seq 20); do
    bash stand/mknod-input.sh >>"$RUN-mknod.log" 2>&1
    SRC=$(source_path)
    [ -n "$SRC" ] && break
    sleep 0.5
done
[ -n "$SRC" ] || { echo "FATAL: no evdev node named $SRC_NAME -- load uinput/evdev (stand/setup-host.sh)"; exit 9; }
echo "source: $SRC"

echo "--- 4. daemon config: our device, X11 branch ---"
cp xswitcher.conf "$CONF"
sed -i "/^\[ScanDevices\]/,/^\[/ s|^\s*Test *=.*|Test = \"$SRC\"|" "$CONF"
sed -i "s/^\s*BypassX = true/ BypassX = false/" "$CONF"
grep -Eq "^\s*BypassX = false" "$CONF" || { echo "FATAL: BypassX not switched to the X11 branch"; exit 9; }

echo "--- 5. G2: without the bridge X sees nothing of the source ---"
# The eye takes X input focus; the bridge is deliberately not running yet.
timeout 12 "$EYE_BIN" >"$EYE" 2>&1 &
pidfile eye $!
sleep 1
python3 "$RUN-source_key.py" play "$SRC" A,A,A
sleep 2
EYE_BEFORE=$(wc -l < "$EYE")
stop_one eye; wait_gone eye
check "G2 evdev keystrokes are invisible to X (saw $EYE_BEFORE)" \
      "$([ "$EYE_BEFORE" = "0" ] && echo 0 || echo 1)"

echo "--- 6. build the daemon ---"
# The stamp comes from the caller (stand/run-gui-stand.sh computes it on the host): the mounted
# checkout can already be dirty or on another commit by the time this runs. A screenshot of an
# untagged build cannot be tied back to a commit.
COMMIT="${GUISTAND_COMMIT:-$(git rev-parse --short HEAD 2>/dev/null || echo )}"
VERSION="${GUISTAND_VERSION:-$(git describe --always --tags --dirty 2>/dev/null || echo )}"
[ -n "$COMMIT" ] && [ -n "$VERSION" ] || {
    echo "FATAL: no build stamp -- pass GUISTAND_COMMIT=<short sha> and GUISTAND_VERSION=<describe>"
    exit 9; }
LDFLAGS="-X xswitcher/internal/about.Commit=$COMMIT -X xswitcher/internal/about.Version=$VERSION"
# Rebuild when the stamp changed, otherwise the binary on screen is from some other commit.
STAMP="$COMMIT $VERSION"
if [ ! -x "$DAEMON_BIN" ] || [ "$(cat "$RUN-stamp" 2>/dev/null)" != "$STAMP" ]; then
    go build -ldflags "$LDFLAGS" -o "$DAEMON_BIN" ./src/ >"$RUN-build-daemon.log" 2>&1 || \
        { echo "FATAL: go build ./src/"; tail -10 "$RUN-build-daemon.log"; exit 9; }
    echo "$STAMP" > "$RUN-stamp"
fi
echo "daemon: $DAEMON_BIN (Commit=$COMMIT Version=$VERSION)"

echo "--- 7. daemon + bridge ---"
"$DAEMON_BIN" -v -c "$CONF" >"$LOG" 2>&1 &
pidfile daemon $!
sleep 2
kill -0 "$(cat "$RUN-daemon.pid")" 2>/dev/null || { echo "FATAL: the daemon died at start"; tail -20 "$LOG"; exit 9; }
bash stand/mknod-input.sh >>"$RUN-mknod.log" 2>&1   # expose the node the daemon just created
"$BRIDGE_BIN" --name "keybd interface" >"$BRIDGE" 2>&1 &
pidfile bridge $!
sleep 1
grep -q -- "-> XTEST keycode+8" "$BRIDGE"; check "G3 the bridge attached to the daemon's own keyboard" "$?"

echo "--- 8. G4-G6: type a Russian word on the English layout ---"
$XGROUP set 0 >/dev/null 2>&1
BEFORE=$($XGROUP get)
rm -f "$EYE"
timeout 25 "$EYE_BIN" >"$EYE" 2>&1 &
pidfile eye $!
sleep 1
# p f y d f on the US layout is "з а н в а" on the Russian one; PAUSE triggers RetypeWord.
python3 "$RUN-source_key.py" play "$SRC" P,F,Y,D,F,PAUSE
sleep 3
AFTER=$($XGROUP get)
echo "  group: $BEFORE  ->  $AFTER"
echo "$AFTER" | grep -q "locked 1"; check "G4 the live server moved to group 1" "$?"
grep -q "XkbLockGroup(1) answered 1" "$LOG"; check "G4 the daemon moved it through XkbLockGroup" "$?"

BS=$(grep -c "name=BackSpace" "$EYE")
CY=$(grep -c "name=Cyrillic_" "$EYE")
KEYS=$(grep -c "^key " "$EYE")
check "G5 the word was wiped (5 BackSpace, saw $BS)" "$([ "$BS" = "5" ] && echo 0 || echo 1)"
check "G5 the word came back as Cyrillic (5 keysyms, saw $CY)" "$([ "$CY" = "5" ] && echo 0 || echo 1)"
SEQ=$(grep "name=Cyrillic_" "$EYE" | sed -E 's/.*name=([^ ]+).*/\1/' | tr "\n" " ")
echo "  keysym sequence: $SEQ"
[ "$SEQ" = "Cyrillic_ze Cyrillic_a Cyrillic_en Cyrillic_ve Cyrillic_a " ]
check "G5 p->ze f->a y->en d->ve f->a (the Russian mapping itself)" "$?"
check "G6 no loop: exactly 10 KeyPress in the eye, saw $KEYS" "$([ "$KEYS" = "10" ] && echo 0 || echo 1)"

echo "--- 9. G8: what the daemon itself reports ---"
IPC=$(python3 - "$SOCK" <<'PY'
import json, socket, sys
s = socket.socket(socket.AF_UNIX); s.settimeout(3); s.connect(sys.argv[1])
s.sendall(json.dumps({"cmd": "status"}).encode() + b"\n")
print(s.recv(65536).decode())
PY
)
echo "  $IPC"
echo "$IPC" | grep -q '"word_buffer":"pfydf"'; check "G8 IPC word_buffer is the typed word" "$?"
echo "$IPC" | grep -q '"word_group":1';          check "G8 IPC word_group is the wrong layout (1)" "$?"

echo "--- 10. G7: the GUI window ---"
if [ ! -x "$GUI_BIN" ] || [ "$(cat "$RUN-stamp-gui" 2>/dev/null)" != "$STAMP" ]; then
    go build -ldflags "$LDFLAGS" -o "$GUI_BIN" ./cmd/gui/ >"$RUN-build-gui.log" 2>&1 || \
        { echo "FATAL: go build ./cmd/gui/"; tail -10 "$RUN-build-gui.log"; exit 9; }
    echo "$STAMP" > "$RUN-stamp-gui"
fi
"$GUI_BIN" >"$RUN-gui.log" 2>&1 &
pidfile gui $!
sleep 12
WINS=$(xwininfo -root -tree 2>/dev/null | grep -ci xswitcher)
import -window root "$RUN-shot.png" 2>/dev/null
SIZE=$(stat -c %s "$RUN-shot.png" 2>/dev/null || echo 0)
check "G7 the GUI maps a window on :$DISPLAY_NO ($WINS)" "$([ "$WINS" -gt 0 ] && echo 0 || echo 1)"
check "G7 screenshot captured ($SIZE bytes)" "$([ "$SIZE" -gt 5000 ] && echo 0 || echo 1)"

stop_all
echo ""
if [ "$TOTAL" != "$EXPECT" ]; then
    say "checks run" "$TOTAL (expected $EXPECT)"
    echo "GUI-STAND: CHECK COUNT MISMATCH"
    exit 8
fi
if [ "$FAILED" = "0" ]; then
    say "checks run" "$TOTAL / GUI-STAND: ALL GREEN"
    exit 0
fi
say "checks run" "$TOTAL / GUI-STAND: $FAILED FAILED"
exit 7
