#!/usr/bin/python3
"""Verify the EV_KEY stream xswitcher emits on its own virtual keyboard.

usage: analyze.py <tsv> <word-csv> <shortcut-digit>
  <tsv>              rows: time<TAB>code<TAB>value, as written by sniff.py
  <word-csv>         the word that was typed, e.g. "H,E,L,L,O"; "" for no word
  <shortcut-digit>   which [Wayland] Layout<N> shortcut is expected

The expectations are derived from the word, so the same file checks every phase of
the stand: BackSpace count == word length, then the word replayed down/up.
Exits non-zero when any check fails.
"""
import sys

from evdev import ecodes as e

BACKSPACE, META = 14, 125
TRIGGER, TRIGGER_UP = e.KEY_PAUSE, 0

events = []
with open(sys.argv[1]) as fh:
    for line in fh:
        t, code, value = line.split("\t")
        events.append((float(t), int(code), int(value)))

word = [t for t in (sys.argv[2].split(",") if len(sys.argv) > 2 and sys.argv[2] else []) if t]
digit = getattr(e, "KEY_%s" % sys.argv[3]) if len(sys.argv) > 3 else e.KEY_2

SHORTCUT = [(META, 1), (digit, 1), (digit, 0), (META, 0)]
RETYPE = [(getattr(e, "KEY_%s" % k), v) for k in word for v in (1, 0)]


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

bs_downs = streams.get(BACKSPACE, []).count(1)

if word:
    check("E1 emitted stream is not empty", len(events) > 0, "%d events" % len(events))
    check("E3 %d BackSpace presses for '%s'" % (len(word), "".join(word)), bs_downs == len(word),
          "got %d" % bs_downs)
    sc_end = find(SHORTCUT)
    check("E4 Win+%s layout shortcut emitted" % sys.argv[3], sc_end >= 0)
    bs_first = next((i for i, ev in enumerate(events) if ev[1] == BACKSPACE), -1)
    check("E5 shortcut precedes the BackSpaces", sc_end >= 0 and bs_first > sc_end,
          "shortcut@%d bs@%d" % (sc_end, bs_first))
    last_bs = max((i for i, ev in enumerate(events) if ev[1] == BACKSPACE), default=-1)
    check("E6 word '%s' retyped after the wipe" % "".join(word), find(RETYPE, last_bs + 1) >= 0)
else:
    check("E1 emitted stream is not empty", len(events) > 0, "%d events" % len(events))
    check("E3 no BackSpace without a word to wipe", bs_downs == 0, "got %d" % bs_downs)
    check("E4 Win+%s layout shortcut emitted" % sys.argv[3], find(SHORTCUT) >= 0)
    check("E6 no retype without a word", find(RETYPE) < 0)

check("E2 no value>2 rows", all(v in (0, 1, 2) for _, _, v in events))
pours = streams.get(TRIGGER, [])
check("E7 trigger PAUSE not retyped", not pours, "saw %d" % len(pours))
stuck = [code for code, vals in streams.items() if len(vals) and vals[-1] == 1]
check("E8 no key left pressed at the end", not stuck, "stuck=%s" % stuck)

print("--- raw stream ---")
print(" ".join("%d:%d" % (c, v) for _, c, v in events[:80]))
sys.exit(1 if fails else 0)
