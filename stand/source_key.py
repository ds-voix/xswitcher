#!/usr/bin/python3
"""Virtual evdev keyboard for the stand: the "human hand" xswitcher reads.

usage:
  source_key.py hold                 # create the device and keep it alive
  source_key.py play <dev-path> <CSV of key tokens>   # tap keys on an existing device
"""
import sys
import time

from evdev import InputDevice, UInput, ecodes as e

DEV_NAME = "stand-source-keyboard"
CAPS = {e.EV_KEY: list(range(0, 256))}


def make():
    return UInput(CAPS, name=DEV_NAME, bustype=e.BUS_USB)


def code_of(token):
    name = token if token.startswith("KEY_") else "KEY_" + token
    code = getattr(e, name, None)
    if code is None:
        raise SystemExit("unknown key token: %s" % token)
    return code


def play(ui, tokens, delay=0.03):
    for token in tokens:
        if token == "#":          # separator, no event
            continue
        rep = 1
        if token.startswith("x"):  # xN:key repeats the tap N times
            n, token = token.split(":", 1)
            rep = int(n[1:])
        code = code_of(token)
        for _ in range(rep):
            ui.write(e.EV_KEY, code, 1)
            ui.syn()
            time.sleep(delay)
            ui.write(e.EV_KEY, code, 0)
            ui.syn()
            time.sleep(delay)


if __name__ == "__main__":
    action = sys.argv[1]
    if action == "hold":  # keep the device alive, xswitcher reads it
        ui = make()
        print("source ready", flush=True)
        while True:
            time.sleep(1)
    elif action == "play":
        dev = InputDevice(sys.argv[2])
        play(dev, sys.argv[3].split(","))
    else:
        raise SystemExit("unknown action: %s" % action)
