#!/bin/bash
# End-to-end stand: real evdev source device -> xswitcher -> its own uinput keyboard.
# Phases 5-11 check the Wayland branch (the shipped config replays shortcuts, BypassX=true) on
# the emitted EV_KEY stream; phase 12 checks the X11 branch (XkbLockGroup) on the live server;
# phase 13 checks that re-creating an already attached device node cannot start a second reader;
# phase 14 checks that a device the scan skips does not keep its descriptor open.
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
# (14 lifecycle/stream + 4 SEQ tail + 19 X11 branch + 6 single-reader + 1 descriptor).
EXPECT=44

say() { printf '%-46s %s\n' "$1" "${2:-}"; }
check() { # check <name> <0|1>
    TOTAL=$((TOTAL + 1))
    if [ "$2" = "0" ]; then say "$1" "PASS"; else say "$1" "FAIL"; FAILED=1; fi
}

source_path() { # the node of the stand's own source keyboard, whatever eventN it got
    python3 - <<'PY'
import sys, evdev
for p in evdev.list_devices():
    try:
        d = evdev.InputDevice(p)
    except Exception as err:      # a node whose kernel device is already gone
        print("skip %s: %s" % (p, err), file=sys.stderr)
        continue
    if d.name == "stand-source-keyboard":
        print(d.path); break
PY
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
SPID=$!
# The node evdev creates for the uinput device appears asynchronously, and --privileged does
# not add devices created after the container started, so mknod has to be retried until
# /sys/class/input shows it. Measured: with uinput and evdev both loaded and /dev/uinput
# present, one fixed 1.5 s sleep let the stand abort here (stand-run-15.log).
SRC=""
for i in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
    SRC=$(source_path)
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

echo "--- 11. a variable-length SEQ tail is reported and disabled (P1-5) ---"
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
# Reporting it is not enough: the rule must not run. Its EXTRA -- the number of trailing events
# RetypeWord leaves alone -- is read off the match, and a match of unfixed length makes that
# number a lie. Measured with the guard off (see the git history of this check): the same stream
# fired the chain and the virtual keyboard emitted only the layout shortcut, four events
# (125:1, 3:1, 3:0, 125:0) -- no BackSpace and no replay, so the word stayed in the wrong layout.
# With the guard the shortcut emits nothing at all.
TSVQ=/tmp/keybdq.tsv
rm -f "$TSVQ"
python3 stand/sniff.py "keybd interface" "$TSVQ" 12 >/tmp/sniffq.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "H,E,L,L,O,PAUSE"
sleep 3
[ ! -s "$TSVQ" ]; check "A13 the unprovable rule emits nothing" "$?"
[ -s "$TSVQ" ] && { echo "  --- what the unprovable rule emitted ---"; head -10 "$TSVQ" | sed 's/^/  /'; }
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

# X13: the checkLanguageId gate must not merely skip the unmanaged group, it has to drop the
# buffers too. A WORD typed on a managed group and left unretyped (no trigger yet) is stale by the
# time the user comes back from an unmanaged group: the app already holds those chars, and the
# letters typed during the unmanaged sojourn were never tracked. Firing the switch trigger after
# the return would retype that stale WORD and duplicate it. With the gate dropping buffers, the
# return triggers an empty retype -- nothing is emitted. Negative control: revert the gate and the
# stale "GAP" is backspaced and retyped, so this sniffer stops being empty.
/tmp/xgroup set 0 >/dev/null 2>&1                      # a known managed start
python3 stand/source_key.py play "$SRC" "ENTER"        # Drop: begin from an empty buffer
sleep 0.5
python3 stand/source_key.py play "$SRC" "G,A,P"        # accumulate a WORD, no trigger yet
TSV11=/tmp/keybd11.tsv
python3 stand/sniff.py "keybd interface" "$TSV11" 15 >/tmp/sniff11.log 2>&1 &
sleep 1
/tmp/xgroup set 2 >/dev/null 2>&1                      # external jump to the unmanaged group
python3 stand/source_key.py play "$SRC" "W"            # gate hit while GAP is still pending
sleep 0.5
/tmp/xgroup set 0 >/dev/null 2>&1                      # back to a managed group
python3 stand/source_key.py play "$SRC" "PAUSE"        # RetypeWord trigger on an (expected) empty buffer
sleep 3
[ ! -s "$TSV11" ]; check "X13 stale word dropped by the unmanaged gate" "$?"

# X14: the issue #13 case. The focus moves to a DIFFERENT real window (B, "terminal") and back to
# A ("editor") in the middle of a word -- yakuake's Alt+` dropdown in the report. The old code held
# one global buffer and dropped it on any window change, so A's half-typed word was lost and the
# next switch trigger retype errored. With per-window buffers, save A's word on leaving, restore it
# on return: the retype after the detour must still wipe and retype exactly "W,O,R,D".
python3 stand/source_key.py play "$SRC" "ENTER"     # NewSentence: begin from a clean buffer
sleep 0.4
# /tmp/x11.log is cumulative across the whole phase, and X13's intentional empty retype already
# wrote a "RetypeWord error" into it. Count before and after so X14 asserts only that the window
# detour introduces no NEW error, instead of tripping over an earlier sub-check's benign log line.
ERR_BEFORE=$(grep -c "RetypeWord error" /tmp/x11.log)
TSV12=/tmp/keybd12.tsv
python3 stand/sniff.py "keybd interface" "$TSV12" 25 >/tmp/sniff12.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "W,O"       # start a word in window A (focus is on A)
sleep 0.4
kill -USR1 $MW; sleep 0.5                           # focus -> window B, a different window id
python3 stand/source_key.py play "$SRC" "Q,U"       # a distinct word typed in B
sleep 0.4
kill -USR2 $MW; sleep 0.5                           # focus -> back to window A
python3 stand/source_key.py play "$SRC" "R,D,PAUSE" # finish A's word; PAUSE fires RetypeWord
sleep 3
ERR_AFTER=$(grep -c "RetypeWord error" /tmp/x11.log)
[ "$ERR_AFTER" = "$ERR_BEFORE" ]; check "X14 no new RetypeWord error across a real window switch" "$?"
python3 stand/analyze.py "$TSV12" "W,O,R,D" none; check "X14 window A's word survived the detour through B" "$?"
[ -s /tmp/x11.log ] && { echo "  x11 branch output:"; grep -E "Language|RETYPE|BACKSPACE" /tmp/x11.log | tail -6 | sed 's/^/    /'; }
kill $X11PID 2>/dev/null

# X15-X17: the unmanaged-language gate and Switch() have to agree about which groups are ours.
# The gate read only the global [ActionKeys] Layouts, while Switch()/Layout() walk the Layouts of
# the concrete action -- so a group reachable only through an action was treated as an "extra
# language": the gate dropped the buffers and returned before Switch() ever ran. The switch the
# config asked for landed in a layout where nothing is collected.
# Negative control, measured with collectManagedLayouts() removed from config(): the sniffer file
# stays empty (no BackSpace, no replay) and /tmp/managed.log prints no "Language:" line at all,
# because Language(-1) is only ever called from Switch().
sleep 1
sed '/^\[Action\.CyclicSwitch\]/,/^\[/ s/^\s*Layouts = \[0, 1\]/       Layouts = [0, 1, 2]/' /tmp/x11.conf > /tmp/managed.conf
WIDENED=$(grep -c "Layouts = \[0, 1, 2\]" /tmp/managed.conf)
KEPT=$(grep -c "Layouts = \[0, 1\]" /tmp/managed.conf)
[ "$WIDENED" = "1" ] && [ "$KEPT" = "1" ]; check "X15 only the action Layouts widened, global list keeps [0, 1]" "$?"
[ "$WIDENED" = "1" ] && [ "$KEPT" = "1" ] || say "  widened=$WIDENED kept=$KEPT:" "the next two checks prove nothing"
/tmp/xgroup set 2 >/dev/null 2>&1          # German: unmanaged by [ActionKeys], managed via the action
/tmp/xswitcher -v -c /tmp/managed.conf >/tmp/managed.log 2>&1 &
MG_PID=$!
sleep 2
TSV13=/tmp/keybd13.tsv
rm -f "$TSV13"
python3 stand/sniff.py "keybd interface" "$TSV13" 15 >/tmp/sniff13.log 2>&1 &
SNIFF13=$!
sleep 1
python3 stand/source_key.py play "$SRC" "B,Y,E,PAUSE"
sleep 3
python3 stand/analyze.py "$TSV13" "B,Y,E" none; check "X16 an action-named group is served, not dropped" "$?"
[ -s "$TSV13" ] || { echo "  --- managed daemon output ---"; head -20 /tmp/managed.log | sed 's/^/  /'; }
/tmp/xgroup get until 0 3 >/dev/null; SRV13=$?
grep -q "Language: -1 >> 2" /tmp/managed.log && grep -q "Language: 0 >> 0" /tmp/managed.log; LOG13=$?
[ "$SRV13" = "0" ] && [ "$LOG13" = "0" ]; check "X17 Switch() wrapped group 2 to 0 on the live server" "$?"
kill $SNIFF13 2>/dev/null
kill $MG_PID 2>/dev/null

echo "--- 13. the source node re-created under the same path stays single-reader ---"
# The inotify handler attaches whatever node appears in /dev/input, and an open fd survives the
# unlink of its node. Re-creating eventN (a udev reload, a hand-made mknod) therefore left the
# old events() goroutine reading the very same kernel device while a second one started next to
# it; both pushed every keystroke into keyboardEvents, the press/release counters desynced and
# RetypeWord leaked its own trigger into the output. Measured before the fix: 14 emitted rows
# instead of 12, the stream ending in "119:1 119:0", and the log saying
# "RetypeWord warning: found pushed keys after retyping was done!".
NODE=$(basename "$SRC")
[ -e "/sys/class/input/$NODE/dev" ] || { echo "FATAL: no /sys/class/input/$NODE/dev to rebuild $SRC from"; exit 9; }
MAJMIN=$(cat "/sys/class/input/$NODE/dev")        # measured "13:64" for event0
/tmp/xswitcher -v -c "$CONF" >/tmp/dup.log 2>&1 &
DUPPID=$!
sleep 2
bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1  # this daemon made a new uinput node
TSV8=/tmp/keybd8.tsv
python3 stand/sniff.py "keybd interface" "$TSV8" 25 >/tmp/sniff8.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "H,I,PAUSE"
sleep 3
python3 stand/analyze.py "$TSV8" "H,I" 2 >/tmp/a8.log 2>&1; A8=$?
check "D1 baseline: one reader, a clean stream" "$A8"
[ "$A8" != "0" ] && { echo "  --- baseline analysis ---"; tail -12 /tmp/a8.log | sed 's/^/  /'; }

echo "  rebuilding $SRC (unlink + mknod c $MAJMIN; the kernel device itself never went away)"
rm -f "$SRC"
mknod "$SRC" c "${MAJMIN%%:*}" "${MAJMIN##*:}"
sleep 3
grep -n "New input device" /tmp/dup.log | tail -2 | sed 's/^/  /'
python3 stand/source_key.py play "$SRC" "ENTER"  # Drop key: the next run starts from an empty buffer
sleep 0.5
TSV9=/tmp/keybd9.tsv
python3 stand/sniff.py "keybd interface" "$TSV9" 25 >/tmp/sniff9.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$SRC" "H,I,PAUSE"
sleep 3
python3 stand/analyze.py "$TSV9" "H,I" 1 >/tmp/a9.log 2>&1; A9=$?
check "D4 the stream after the rebuild is still clean" "$A9"
[ "$A9" != "0" ] && { echo "  --- after-rebuild analysis ---"; tail -12 /tmp/a9.log | sed 's/^/  /'; }
ATTACH_HITS=$(grep -c "  $SRC:" /tmp/dup.log)
[ "$ATTACH_HITS" = "1" ]; check "D2 $SRC attached exactly once after the rebuild (hits=$ATTACH_HITS)" "$?"
grep -q "found pushed keys" /tmp/dup.log; PUSHED=$?
[ "$PUSHED" != "0" ]; check "D3 no key left pushed (the reader counters stayed in sync)" "$?"
[ "$PUSHED" = "0" ] && { echo "  desynchronised state after the rebuild:"; grep -E "RetypeWord|RETYPE" /tmp/dup.log | tail -4 | sed 's/^/  /'; }

# A path registry must not turn into "never attach again": destroying the kernel device makes
# the reader leave with an error, and a device that shows up afterwards has to be read once more.
kill $SPID 2>/dev/null
for i in $(seq 20); do grep -q "Closing device \"stand-source-keyboard\"" /tmp/dup.log && break; sleep 0.3; done
grep -q "Closing device \"stand-source-keyboard\"" /tmp/dup.log; CLOSED=$?
[ "$CLOSED" = "0" ]; check "D5 the reader left when its device was destroyed" "$?"
[ "$CLOSED" != "0" ] && echo "  no Closing device line in /tmp/dup.log"
# The container runs without udev, so the node of the destroyed device would stay behind and
# break the next one: python-evdev's UInput() scans /dev/input/event* and dies on an
# ENODEV node, and mknod-input.sh skips names that already exist. Measured: with the stale
# /dev/input/event0 left in place the new source keyboard was never created at all.
rm -f "$SRC"
python3 stand/source_key.py hold >/tmp/source2.log 2>&1 &
SPID2=$!
NEWSRC=""
for i in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
    NEWSRC=$(source_path 2>/tmp/source_path.err)
    [ -n "$NEWSRC" ] && break
    sleep 0.5
done
echo "  new source device: ${NEWSRC:-none} (was $SRC)"
[ -n "$NEWSRC" ] || {
    echo "  --- the restarted source was not found: nodes ---"
    ls -l /dev/input/ 2>&1 | sed 's/^/  /'
    echo "  --- sysfs ---"
    for d in /sys/class/input/event*; do
        [ -e "$d" ] && echo "  $d name=$(cat "$d/device/name" 2>/dev/null) dev=$(cat "$d/dev" 2>/dev/null)"
    done
    echo "  --- hold output ---"; cat /tmp/source2.log 2>/dev/null | sed 's/^/  /'
    echo "  --- source_path stderr ---"; head -5 /tmp/source_path.err | sed 's/^/  /'
}
TSV10=/tmp/keybd10.tsv
python3 stand/sniff.py "keybd interface" "$TSV10" 25 >/tmp/sniff10.log 2>&1 &
sleep 1
python3 stand/source_key.py play "$NEWSRC" "ENTER" 2>/dev/null
sleep 0.5
python3 stand/source_key.py play "$NEWSRC" "H,I,PAUSE"
sleep 3
python3 stand/analyze.py "$TSV10" "H,I" 2 >/tmp/a10.log 2>&1; A10=$?
check "D6 a device that reappears is attached and switches" "$A10"
[ "$A10" != "0" ] && { echo "  --- reappearing-device analysis ---"; tail -12 /tmp/a10.log | sed 's/^/  /'; }
kill $SPID2 2>/dev/null
kill $DUPPID 2>/dev/null

echo "--- 14. a bypassed device leaves no descriptor behind ---"
# connectEvents() opens every node it inspects and only the devices it starts a reader for keep
# that descriptor: the BypassRE branch and the "no EV_KEY capability" branch used to hand nothing
# back. With the collector running the os.File finalizer hides this (measured: both skipped
# devices were gone from /proc/<pid>/fd a second later), so the daemon runs with GOGC=off here and
# every unclosed fd stays visible. Measured before the fix: 4 event fds held, 3 of them the
# bypassed cameras; with the explicit Close, 1 (the attached source).
python3 stand/source_key.py hold >/tmp/source3.log 2>&1 &
SPID3=$!
SRC3=""
for i in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
    SRC3=$(source_path 2>/dev/null)
    [ -n "$SRC3" ] && break
    sleep 0.5
done
[ -n "$SRC3" ] || { echo "FATAL: phase 14 has no source device"; tail -5 /tmp/source3.log; exit 9; }
python3 - >/tmp/cams.log 2>&1 <<'PY' &
import time
from evdev import UInput, ecodes as e
uis = [UInput({e.EV_KEY: [e.KEY_A, e.KEY_B]}, name="Integrated Camera %d" % i, bustype=e.BUS_USB)
       for i in range(3)]
print("cameras ready: %d" % len(uis), flush=True)
while True:
    time.sleep(1)
PY
CAMPID=$!
CAMS=0
for i in $(seq 20); do
    bash stand/mknod-input.sh >>/tmp/mknod.log 2>&1
    CAMS=$(python3 - <<'PY'
import evdev
n = 0
for p in evdev.list_devices():
    try:
        d = evdev.InputDevice(p)
    except Exception:
        continue
    if "Camera" in d.name:
        n += 1
print(n)
PY
)
    [ "$CAMS" = "3" ] && break
    sleep 0.5
done
echo "  camera nodes visible: $CAMS of 3, source: $SRC3"
cp "$CONF" /tmp/fd.conf
sed -i "/^\[ScanDevices\]/,/^\[/ s|^\s*Test *=.*|Test = \"$SRC3\"|" /tmp/fd.conf
# The shipped Respawn = -1 is what makes this phase possible: serve() schedules a self-respawn
# when the Test node is younger than that window, and here the node was just made by hand. A fork
# would replace the very process whose /proc entries are being counted, so the config keeps -1
# (appending a second Respawn key is a TOML parse error and the daemon simply refuses to start --
# measured on the first run of this phase).
grep -nE "Respawn|Test =|Bypass =" /tmp/fd.conf | head -4 | sed 's/^/  /'
GOGC=off /tmp/xswitcher -v -c /tmp/fd.conf >/tmp/fd.log 2>&1 &
FDPID=$!
for i in $(seq 40); do
    HITS=$(grep -cE "^- /dev/input/event[0-9]+:[[:space:]]+Integrated Camera" /tmp/fd.log)
    [ "${HITS:-0}" = "3" ] && break
    sleep 0.5
done
[ -d "/proc/$FDPID" ] || { echo "FATAL: the GOGC=off daemon of phase 14 is gone"; head -5 /tmp/fd.log | sed 's/^/  /'; exit 9; }
sleep 1
HELD=$(ls -l /proc/$FDPID/fd 2>/dev/null | grep -oE "/dev/input/event[0-9]+" | sort -u | wc -l)
echo "  bypassed devices in the log: ${HITS:-0} of 3, event descriptors held: $HELD"
[ "$HELD" = "1" ]; check "F1 a bypassed device is closed again (held=$HELD, source only)" "$?"
[ "$HELD" != "1" ] && ls -l /proc/$FDPID/fd 2>/dev/null |
    grep -oE "/dev/input/event[0-9]+" | sort | uniq -c | sed 's/^/  /'
kill $FDPID $CAMPID $SPID3 2>/dev/null

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
