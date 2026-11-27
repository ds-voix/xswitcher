#!/usr/bin/python3
"""Record the evdev events xswitcher emits on its own virtual keyboard.

usage: sniff.py <device-name> <out-tsv> [timeout-sec]
Output rows: <monotonic-sec>\t<code>\t<value>   (EV_KEY only)
"""
import sys
import time

import evdev

WANT = sys.argv[1] if len(sys.argv) > 1 else "keybd interface"
OUT = sys.argv[2] if len(sys.argv) > 2 else "/tmp/out.tsv"
DEADLINE = time.time() + float(sys.argv[3] if len(sys.argv) > 3 else 20)

dev = None
while dev is None and time.time() < DEADLINE:
    for path in evdev.list_devices():
        try:
            d = evdev.InputDevice(path)
        except OSError:
            continue
        if d.name == WANT:
            dev = d
            break
    if dev is None:
        time.sleep(0.2)

if dev is None:
    sys.stderr.write("sniffer: device %r never appeared\n" % WANT)
    sys.exit(1)

sys.stderr.write("sniffer: recording from %s (%s)\n" % (dev.path, dev.name))
t0 = time.monotonic()
with open(OUT, "w") as fh:
    for ev in dev.read_loop():
        if ev.type != evdev.ecodes.EV_KEY:
            continue
        fh.write("%.3f\t%d\t%d\n" % (time.monotonic() - t0, ev.code, ev.value))
        fh.flush()
        if time.time() > DEADLINE:
            break
