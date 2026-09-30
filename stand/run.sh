#!/bin/bash
# End-to-end stand: real evdev source device -> xswitcher -> its own uinput keyboard.
# The behaviour is checked on the emitted EV_KEY stream, not in X: the shipped default
# config switches layouts by replaying shortcuts ([Wayland] BypassX=true), and container
# Xvfb cannot host a two-group keymap anyway.
# Requires: docker run --privileged, uinput module loaded on the host kernel.
set -u
cd /w

LOG=/tmp/xswitcher.log
CONF=/tmp/stand.conf
TSV=/tmp/keybd.tsv
FAILED=0

say() { printf '%-46s %s\n' "$1" "${2:-}"; }
check() { # check <name> <0|1>
    if [ "$2" = "0" ]; then say "$1" "PASS"; else say "$1" "FAIL"; FAILED=1; fi
}

echo "--- 1. device nodes ---"
bash stand/mknod-input.sh >/tmp/mknod.log 2>&1
[ -e /dev/uinput ] || { echo "FATAL: no /dev/uinput (host kernel lacks CONFIG_INPUT_UINPUT or module not loaded)"; exit 9; }

echo "--- 2. X server ---"
# Only a display to connect to: XOpenDisplay() is unconditional in main().
Xvfb :99 -screen 0 1024x768x16 >/tmp/xvfb.log 2>&1 &
for i in $(seq 20); do [ -e /tmp/.X11-unix/X99 ] && break; sleep 0.3; done
export DISPLAY=:99
setxkbmap -layout us >/dev/null 2>&1

echo "--- 3. source keyboard ---"
python3 stand/source_key.py hold >/tmp/source.log 2>&1 &
sleep 1.5
bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
SRC=$(python3 - <<'PY'
import evdev
for p in evdev.list_devices():
    d = evdev.InputDevice(p)
    if d.name == "stand-source-keyboard":
        print(d.path); break
PY
)
[ -n "${SRC:-}" ] || { echo "FATAL: no /dev/input/event* for the source device -- the host kernel is missing evdev/uinput; run stand/setup-host.sh"; cat /tmp/mknod.log; exit 9; }
echo "source device: $SRC"

echo "--- 4. config + build ---"
cp xswitcher.conf "$CONF"
# "Test" exists twice in the shipped config ([ScanDevices] and [Keys]); only the
# [ScanDevices] one names the mandatory device.
sed -i "/^\[ScanDevices\]/,/^\[/ s|^\s*Test *=.*|Test = \"$SRC\"|" "$CONF"
grep -n 'Test =\|BypassX' "$CONF" | sed 's/^/  /'
export GOFLAGS=-buildvcs=false
go build -o /tmp/xswitcher ./src/ >/tmp/build.log 2>&1; BUILD=$?
grep -v '^go: downloading' /tmp/build.log | head -5
[ -x /tmp/xswitcher ] || { echo "FATAL: build failed (rc=$BUILD)"; exit 9; }
check "B1 go build ./src/" "0"
go vet ./src/ >/tmp/vet.log 2>&1; VET=$?
grep -v '^go: downloading' /tmp/vet.log | head -20
check "B2 go vet ./src/" "$VET"
go test ./... >/tmp/test.log 2>&1; TEST=$?
grep -vE '^(go: |ok)' /tmp/test.log | head -5
check "B3 go test ./... (all packages)" "$TEST"

echo "--- 5. run xswitcher ---"
/tmp/xswitcher -v -c "$CONF" >"$LOG" 2>&1 &
XPID=$!
sleep 2
kill -0 $XPID 2>/dev/null; ALIVE=$?

echo "--- 6. sniff own virtual keyboard, inject 'HELLO' + PAUSE ---"
bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1   # expose the uinput node xswitcher just created
python3 stand/sniff.py "keybd interface" "$TSV" 15 >/tmp/sniff.log 2>&1 &
sleep 1.5
tail -1 /tmp/sniff.log | sed 's/^/  /'
python3 stand/source_key.py play "$SRC" "H,E,L,L,O,PAUSE"
sleep 4

echo "--- 7. assertions: daemon lifecycle ---"
[ "$ALIVE" = "0" ]; check "A1 xswitcher survives the run" "$?"
grep -q "Config error" "$LOG"; CONFIG_BAD=$?
[ "$CONFIG_BAD" != "0" ]; check "A2 shipped config parses" "$?"
if [ "$ALIVE" != "0" ] || [ "$CONFIG_BAD" = "0" ]; then echo "  --- xswitcher output ---"; head -20 "$LOG" | sed 's/^/  /'; fi
grep -q "RetypeWord" "$LOG"; check "A3 RetypeWord action fired" "$?"
grep -Eq "BACKSPACE: *5 *- *0 *= *5" "$LOG"; check "A4 5 BackSpaces computed in log" "$?"
SRC_HITS=$(grep -c "stand-source-keyboard" "$LOG")
[ "$SRC_HITS" = "1" ]; check "A6 source device attached once (hits=$SRC_HITS)" "$?"
grep -q "keybd interface" "$LOG"; SELF_ECHO=$?
# Attached devices are printed with a leading double space, skipped ones with "- ".
grep -Eq "^  /dev/input/event[0-9]+:\s*keybd interface" "$LOG"; SELF_ATTACHED=$?
[ "$SELF_ATTACHED" != "0" ]; check "A7 own uinput device not attached (P1-4)" "$?"
[ "$SELF_ECHO" = "0" ]; check "A8 own uinput device explicitly skipped in scan" "$?"

echo "--- 8. assertions: emitted key stream ---"
[ -s "$TSV" ] || { say "TSV empty -- sniffer log:"; tail -3 /tmp/sniff.log | sed 's/^/  /'; FAILED=1; }
python3 stand/analyze.py "$TSV"; check "E-series key-stream checks" "$?"

echo "--- detail ---"
[ "$SELF_ATTACHED" = "0" ] && echo "  P1-4 reproduced: xswitcher reads its own virtual keyboard:" && grep -En "^  .*keybd interface" "$LOG" | head -4
echo "  devices seen:"; grep -E "^\s+[-x]? */dev/input" "$LOG" | head -8
echo "  retype tail:"; grep -E "RETYPE|BACKSPACE|Language" "$LOG" | tail -6

kill $XPID 2>/dev/null
echo "=================================="
[ "$FAILED" = "0" ] && echo "STAND: ALL GREEN" || echo "STAND: FAILURES PRESENT"
exit $FAILED
