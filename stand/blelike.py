#!/usr/bin/python3
"""Virtual evdev keyboard that also reports absolute axes (issue #12 fixture).

Real BLE keyboards often expose EV_ABS for battery level or a media surface alongside
EV_KEY. Upstream xswitcher used to conclude "EV_ABS or EV_REL => this is a mouse" and
either skip the device or open it twice (keyboard + mouse readers racing on one fd),
which is what issue #12 reports as "my Keychron types as a mouse". That heuristic was
removed in upstream commit 5fe6e3a; this fixture is a regression guard that keeps the
fix measured. It advertises EV_KEY + EV_ABS only (no EV_MSC) so the pre-refactor binary
classifies it as a mouse instead of skipping it for an unsupported event type.

usage:
    blelike.py hold                       # create the device and keep it alive
    blelike.py play <dev-path> <CSV>      # tap keys on an existing device
"""
import sys
import time
from evdev import UInput, AbsInfo, ecodes as e

DEV_NAME = "stand-ble-like-keyboard"
AXIS = AbsInfo(value=0, min=0, max=255, fuzz=0, flat=0, resolution=0)
# The key range matches source_key.py on purpose: the kernel drops an EV_KEY write whose
# code the device does not advertise, so a narrower set silently loses the PAUSE trigger.
CAPS = {
    e.EV_KEY: list(range(0, 256)),
    e.EV_ABS: [(e.ABS_X, AXIS), (e.ABS_Y, AXIS)],
}


def make():
    return UInput(CAPS, name=DEV_NAME, bustype=e.BUS_BLUETOOTH)


if __name__ == "__main__":
    action = sys.argv[1]
    if action == "hold":
        ui = make()
        print("ble-like fixture ready", flush=True)
        while True:
            ui.write(e.EV_ABS, e.ABS_X, 128)
            ui.syn()
            time.sleep(1)
    elif action == "play":
        from source_key import play, code_of  # reuse the tap helper
        from evdev import InputDevice
        dev = InputDevice(sys.argv[2])
        tokens = sys.argv[3].split(",")
        for token in tokens:
            if token == "#":
                continue
            rep = 1
            if token.startswith("x"):
                n, token = token.split(":", 1)
                rep = int(n[1:])
            code = code_of(token)
            for _ in range(rep):
                dev.write(e.EV_KEY, code, 1)
                dev.syn()
                time.sleep(0.03)
                dev.write(e.EV_KEY, code, 0)
                dev.syn()
                time.sleep(0.03)
    else:
        raise SystemExit("unknown action: %s" % action)