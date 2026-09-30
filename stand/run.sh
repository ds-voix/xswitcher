#!/bin/bash
# End-to-end stand: real evdev source device -> xswitcher -> its own uinput keyboard.
# Phases 5-11 check the Wayland branch (the shipped config replays shortcuts, BypassX=true) on
# the emitted EV_KEY stream; phase 12 checks the X11 branch (XkbLockGroup) on the live server.
# Requires: docker run --privileged, uinput module loaded on the host kernel.
set -u
cd /w

LOG=/tmp/xswitcher.log
CONF=/tmp/stand.conf
TSV=/tmp/keybd.tsv
FAILED=0
TOTAL=0
# "ALL GREEN" is only meaningful together with the number of checks behind it: a phase
# that aborts early (or gets commented out while debugging) must not look like a pass.
# Keep this in sync with the number of check() calls below
# (14 lifecycle/stream + 3 SEQ tail + 13 X11 branch).
EXPECT=30

say() { printf '%-46s %s\n' "$1" "${2:-}"; }
check() { # check <name> <0|1>
    TOTAL=$((TOTAL + 1))
    if [ "$2" = "0" ]; then say "$1" "PASS"; else say "$1" "FAIL"; FAILED=1; fi
}

echo "--- 1. device nodes ---"
bash stand/mknod-input.sh >/tmp/mknod.log 2>&1
[ -e /dev/uinput ] || { echo "FATAL: no /dev/uinput (host kernel lacks CONFIG_INPUT_UINPUT or module not loaded)"; exit 9; }

echo "--- 2. X server ---"
# Only a display to connect to: XOpenDisplay() is unconditional in main().
# -noreset matters in phase 12: by default Xvfb restores its initial keymap when the last
# client disconnects, and every observer here (setxkbmap, xgroup) is a one-shot client, so a
# loaded multi-group map vanishes between two calls. Measured: the same
# "setxkbmap -layout us,ru" (rc=0) answers "groups 1" with no client left and "groups 2"
# while another client stays connected.
Xvfb :99 -noreset -screen 0 1024x768x16 >/tmp/xvfb.log 2>&1 &
for i in $(seq 20); do [ -e /tmp/.X11-unix/X99 ] && break; sleep 0.3; done
export DISPLAY=:99
setxkbmap -layout us >/dev/null 2>&1

echo "--- 3. source keyboard ---"
python3 stand/source_key.py hold >/tmp/source.log 2>&1 &
# The node evdev creates for the uinput device appears asynchronously, and --privileged does
# not add devices created after the container started, so mknod has to be retried until
# /sys/class/input shows it. Measured: with uinput and evdev both loaded and /dev/uinput
# present, one fixed 1.5 s sleep let the stand abort here (stand-run-15.log).
SRC=""
for i in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
    SRC=$(python3 - <<'PY'
import evdev
for p in evdev.list_devices():
    d = evdev.InputDevice(p)
    if d.name == "stand-source-keyboard":
        print(d.path); break
PY
)
    [ -n "$SRC" ] && break
    sleep 0.5
done
[ -n "${SRC:-}" ] || { echo "FATAL: no /dev/input/event* for the source device after 10s -- the host kernel is missing evdev/uinput; run stand/setup-host.sh"; tail -20 /tmp/mknod.log; exit 9; }
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
python3 stand/analyze.py "$TSV" "H,E,L,L,O" 2; check "E-series key-stream checks" "$?"

echo "--- 9. focus blip in the middle of a word (P1-3) ---"
gcc stand/mkwin.c -o /tmp/mkwin -lX11 >/tmp/mkwin_build.log 2>&1; MKWIN_BUILD=$?
[ "$MKWIN_BUILD" = "0" ] || { echo "FATAL: mkwin build"; head -20 /tmp/mkwin_build.log; exit 9; }
TSV2=/tmp/keybd2.tsv
python3 stand/sniff.py "keybd interface" "$TSV2" 25 >/tmp/sniff2.log 2>&1 &
sleep 1
/tmp/mkwin >/tmp/mkwin.log 2>&1 &
MW=$!
sleep 0.8                       # the editor takes focus: one legitimate buffer drop
python3 stand/source_key.py play "$SRC" "W,O"
sleep 0.2
kill -HUP $MW                   # X answers None/PointerRoot, as during a real window switch
sleep 0.2
python3 stand/source_key.py play "$SRC" "R"   # a key pressed exactly while X has no focus
sleep 0.2
kill -USR2 $MW                  # ... and the very same window is focused again
sleep 0.2
python3 stand/source_key.py play "$SRC" "D,PAUSE"
sleep 3
grep -q "RetypeWord error" "$LOG"; WORD_DROPPED=$?
[ "$WORD_DROPPED" != "0" ]; check "A9 no RetypeWord error after a focus blip" "$?"
[ -s "$TSV2" ] || { say "TSV2 empty -- sniffer log:"; tail -3 /tmp/sniff2.log | sed 's/^/  /'; FAILED=1; }
python3 stand/analyze.py "$TSV2" "W,O,R,D" 1; check "E-series checks (word survived the blip)" "$?"
echo "  focus log:"; sed 's/^/    /' /tmp/mkwin.log

echo "--- 10. lone L_CTRL tap cycles the layout ---"
TSV3=/tmp/keybd3.tsv
python3 stand/sniff.py "keybd interface" "$TSV3" 15 >/tmp/sniff3.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "LEFTCTRL"
sleep 2
python3 stand/analyze.py "$TSV3" "" 2; check "E-series checks (switch only, no wipe)" "$?"

echo "--- 11. a variable-length SEQ tail is reported while parsing (P1-5) ---"
# RetypeWord() derives EXTRA from the length of the matched tail, so ".*PAUSE:1,PAUSE:0"
# matched the whole SeqLength window, EXTRA grew to 12 and the action refused to wipe or
# retype anything: the layout switched and the word stayed in the wrong one. Say it at the
# start, and keep silent about the shipped rules, which are exact.
kill $XPID 2>/dev/null
sleep 0.5
grep -q 'Parse warning:.*RetypeWord' "$LOG"; SHIPPED_WARN=$?
[ "$SHIPPED_WARN" != "0" ]; check "A10 shipped config prints no SEQ warning" "$?"
sed 's/SEQ:(PAUSE:1,PAUSE:0)/SEQ:(.*PAUSE:1,PAUSE:0)/' "$CONF" > /tmp/quant.conf
grep -q 'SEQ:(\.\*PAUSE' /tmp/quant.conf; check "A11 mutated config really differs" "$?"
/tmp/xswitcher -v -c /tmp/quant.conf >/tmp/quant.log 2>&1 &
QPID=$!
sleep 2
grep -q 'Parse warning:.*RetypeWord' /tmp/quant.log; check "A12 variable-length SEQ tail reported" "$?"
[ -s /tmp/quant.log ] || { echo "  --- mutated config output ---"; head -20 /tmp/quant.log | sed 's/^/  /'; }
kill $QPID 2>/dev/null

echo "--- 12. X11 branch: real XkbLockGroup and the unmanaged-language gate ---"
# Three layouts, so that the shipped [ActionKeys] Layouts = [0, 1] leaves group 2 (German)
# unmanaged -- the "extra language (e.g., Chinese)" case the config comment describes. The
# daemon switches through XkbLockGroup() for real here and an independent observer (xgroup)
# reads the server state back; nothing about the group is simulated.
gcc stand/xgroup.c -o /tmp/xgroup -lX11 >/tmp/xgroup_build.log 2>&1; XGROUP_BUILD=$?
[ "$XGROUP_BUILD" = "0" ] || { echo "FATAL: xgroup build"; head -20 /tmp/xgroup_build.log; exit 9; }
setxkbmap -layout us,ru,de >/tmp/setxkbmap.log 2>&1; SX=$?
[ "$SX" = "0" ] || { echo "FATAL: setxkbmap -layout us,ru,de"; cat /tmp/setxkbmap.log; exit 9; }
XGROUP_OUT=$(/tmp/xgroup info 25 2>&1)
echo "$XGROUP_OUT" | grep -q "^groups 3 ctrls 3 key 25 groups 3"; X0=$?
[ "$X0" = "0" ] || echo "  xgroup says: $XGROUP_OUT (DISPLAY=$DISPLAY)"
check "X0 the live server holds three XKB groups" "$X0"
# The group numbers only mean something if the map behind them really is the second layout.
xkbcomp -xkb "$DISPLAY" /tmp/srv.xkb 2>/dev/null
AD02=$(awk '/key <AD02>/ {f=1} f {printf "%s ", $0} f && /};/ {f=0}' /tmp/srv.xkb)
echo "  'w' per group: $AD02"
echo "$AD02" | grep -q 'symbols\[Group2\]= \[ *Cyrillic_'; X1=$?
check "X1 group 2 translates the typed key to Russian" "$X1"
sed 's/^\s*BypassX = true/ BypassX = false/' "$CONF" > /tmp/x11.conf
grep -Eq "^\s*BypassX = false" /tmp/x11.conf; check "X2 config switches through X11" "$?"
/tmp/xgroup set 0 >/dev/null 2>&1          # every run starts on the first layout
/tmp/xswitcher -v -c /tmp/x11.conf >/tmp/x11.log 2>&1 &
X11PID=$!
sleep 2
kill -0 $X11PID 2>/dev/null; ALIVE11=$?
[ "$ALIVE11" = "0" ]; check "X3 daemon survives BypassX=false" "$?"
[ "$ALIVE11" != "0" ] && { echo "  --- x11 output ---"; head -20 /tmp/x11.log | sed 's/^/  /'; }

TSV4=/tmp/keybd4.tsv
python3 stand/sniff.py "keybd interface" "$TSV4" 15 >/tmp/sniff4.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "H,E,L,L,O,PAUSE"
sleep 3
grep -q "Language: -1 >> 0" /tmp/x11.log && grep -q "Language: 1 >> 1" /tmp/x11.log
check "X4 the X11 branch reads group 0, asks for 1" "$?"
/tmp/xgroup get until 1 3 >/dev/null; check "X5 the live server is on group 1 after the action" "$?"
python3 stand/analyze.py "$TSV4" "H,E,L,L,O" none; check "X6 wipe+retype, no shortcut" "$?"

# Switch() walks A.Layouts and wraps to its first entry once the last one is used
# (next >= len(A.Layouts) -> 0), so the second trigger has to land back on group 0.
TSV7=/tmp/keybd7.tsv
python3 stand/sniff.py "keybd interface" "$TSV7" 15 >/tmp/sniff7.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "B,Y,E,PAUSE"
sleep 3
grep -q "Language: -1 >> 1" /tmp/x11.log && grep -q "Language: 0 >> 0" /tmp/x11.log
check "X7 the second trigger reports reading 1, asking for 0" "$?"
/tmp/xgroup get until 0 3 >/dev/null; check "X8 the live server is back on group 0" "$?"
python3 stand/analyze.py "$TSV7" "B,Y,E" none; check "X9 second word wiped and retyped" "$?"

/tmp/xgroup set 2 >/dev/null 2>&1          # German: outside [ActionKeys] Layouts = [0, 1]
/tmp/xgroup get until 2 3 >/dev/null; check "X10 gate precondition: server on group 2" "$?"
TSV5=/tmp/keybd5.tsv
python3 stand/sniff.py "keybd interface" "$TSV5" 15 >/tmp/sniff5.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "W,O,R,D,PAUSE"
sleep 3
[ -s "$TSV5" ]; EXTRA_KEYS=$?
[ "$EXTRA_KEYS" != "0" ]; check "X11 unmanaged group is not intercepted" "$?"
[ "$EXTRA_KEYS" = "0" ] && { echo "  emitted while the group was unmanaged:"; head -3 "$TSV5" | sed 's/^/  /'; }

/tmp/xgroup set 0 >/dev/null 2>&1          # back to a managed group: the gate must let go
python3 stand/source_key.py play "$SRC" "ENTER"   # Drop key, so the stale word cannot interfere
sleep 0.5
TSV6=/tmp/keybd6.tsv
python3 stand/sniff.py "keybd interface" "$TSV6" 15 >/tmp/sniff6.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "H,I,PAUSE"
sleep 3
python3 stand/analyze.py "$TSV6" "H,I" none; check "X12 managed group works again after the gate" "$?"
[ -s /tmp/x11.log ] && { echo "  x11 branch output:"; grep -E "Language|RETYPE|BACKSPACE" /tmp/x11.log | tail -6 | sed 's/^/    /'; }
kill $X11PID 2>/dev/null

echo "--- detail ---"
[ "$SELF_ATTACHED" = "0" ] && echo "  P1-4 reproduced: xswitcher reads its own virtual keyboard:" && grep -En "^  .*keybd interface" "$LOG" | head -4
echo "  devices seen:"; grep -E "^\s+[-x]? */dev/input" "$LOG" | head -8
echo "  retype tail:"; grep -E "RETYPE|BACKSPACE|Language" "$LOG" | tail -6
[ -n "${DUMPLOG:-}" ] && { echo "--- full daemon log ---"; cat "$LOG" | sed 's/^/  /'; }

kill $XPID 2>/dev/null
echo "=================================="
if [ "$TOTAL" != "$EXPECT" ]; then
    say "checks run ($TOTAL of $EXPECT expected)" "FAIL"
    FAILED=1
fi
say "checks run" "$TOTAL"
[ "$FAILED" = "0" ] && echo "STAND: ALL GREEN" || echo "STAND: FAILURES PRESENT"
exit $FAILED
