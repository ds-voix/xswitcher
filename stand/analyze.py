#!/usr/bin/python3
"""Verify the EV_KEY stream xswitcher emits on its own virtual keyboard.

usage: analyze.py <tsv>            # rows: time<TAB>code<TAB>value
Prints one line per check, exits non-zero if any check fails.
"""
import sys

# evdev keycodes
A, E, H, L, O = 30, 18, 35, 38, 24
BACKSPACE, META, DIGIT2, PAUSE = 14, 125, 3, 119

# Expected after the source device played "H,E,L,L,O,PAUSE" with the shipped
# config ([Wayland] BypassX=true, Layout1 = Win+2, Action = CyclicSwitch+RetypeWord).
SHORTCUT = [(META, 1), (DIGIT2, 1), (DIGIT2, 0), (META, 0)]
RETYPE = [(k, v) for k in (H, E, L, L, O) for v in (1, 0)]

events = []
with open(sys.argv[1]) as fh:
    for line in fh:
        t, code, value = line.split("\t")
        events.append((float(t), int(code), int(value)))

def find(seq, start=0):
    """Index of the last element of seq as a subsequence of events, or -1."""
    i = start
    for ev in seq:
        while i < len(events) and (events[i][1], events[i][2]) != ev:
            i += 1
        if i == len(events):
            return -1
        i += 1
    return i - 1

fails = 0

def check(name, ok, detail=""):
    global fails
    if not ok:
        fails += 1
    print("%-52s %s%s" % (name, "PASS" if ok else "FAIL", ("  " + detail) if detail else ""))

streams = {}
for t, code, value in events:
    streams.setdefault(code, []).append(value)

check("E1 emitted stream is not empty", len(events) > 0, "%d events" % len(events))
check("E2 no EV_SYN-like value>2 rows", all(v in (0, 1, 2) for _, _, v in events))

bs = len(streams.get(BACKSPACE, [])) - streams.get(BACKSPACE, [0]).count(0)
# Down-presses are what actually deletes a char.
bs_downs = streams.get(BACKSPACE, []).count(1)
check("E3 exactly 5 BackSpace presses", bs_downs == 5, "got %d" % bs_downs)

sc_end = find(SHORTCUT)
check("E4 Win+2 layout shortcut emitted", sc_end >= 0)

bs_first = next((i for i, e in enumerate(events) if e[1] == BACKSPACE), -1)
check("E5 shortcut precedes the BackSpaces", sc_end >= 0 and bs_first > sc_end,
      "shortcut@%d bs@%d" % (sc_end, bs_first))

last_bs = max((i for i, e in enumerate(events) if e[1] == BACKSPACE), default=-1)
rt_end = find(RETYPE, last_bs + 1)
check("E6 word 'HELLO' retyped after wipe", rt_end >= 0)

pours = streams.get(PAUSE, [])
check("E7 trigger PAUSE not retyped", not pours, "saw %d" % len(pours))

# Each retyped key must be released again: the retype must not leave stuck modifiers.
stuck = [code for code, vals in streams.items()
         if code != BACKSPACE and len(vals) and vals[-1] == 1]
check("E8 no key left pressed at the end", not stuck, "stuck=%s" % stuck)

print("--- raw stream ---")
print(" ".join("%d:%d" % (c, v) for _, c, v in events[:80]))
sys.exit(1 if fails else 0)
