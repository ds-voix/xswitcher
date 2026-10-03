#!/usr/bin/env python3
# Live IPC smoke against a running daemon (stand/smoke.sh brings the whole
# environment up). Covers GUI-PLAN-PUNTO.md Phases 0-2 over the real unix
# socket: ping/status, get_config/set_config/validate, comment preservation,
# rejection of an invalid mod, dotted paths into [Action.*] tables, and the
# whole-array rewrite of [[WindowClasses]].
import json
import socket
import sys

CFG = "/tmp/GuiSmoke.conf"
SOCK = sys.argv[1] if len(sys.argv) > 1 else "/tmp/gui.sock"


def req(s, obj):
    s.sendall((json.dumps(obj) + "\n").encode())
    buf = b""
    while not buf.endswith(b"\n"):
        d = s.recv(4096)
        if not d:
            raise RuntimeError("daemon closed the connection")
        buf += d
    return json.loads(buf)


failures = []


def check(name, cond, detail=""):
    print(f"{name}: {'PASS' if cond else 'FAIL'} {detail}")
    if not cond:
        failures.append(name)


s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.connect(SOCK)

check("ping", req(s, {"cmd": "ping"})["ok"])
st = req(s, {"cmd": "status"})
check("status", st["ok"] and st["pid"] > 0 and st["config"] == CFG, str(st))
# Phase 1: the live-status fields must always be present in the protocol.
check("status layout field", "layout" in st, str(st))
check("status devices list", isinstance(st.get("devices"), list) and len(st["devices"]) > 0, str(st.get("devices")))

r = req(s, {"cmd": "get_config"})
check("get_config", r["ok"] and "ScanDevices" in r["data"], str(sorted(r.get("data", {}))))

r = req(s, {"cmd": "set_config", "data": {"Keyboard.Delay": 9.0, "Wayland.Layout0": ["A", "B"]}})
check("set_config", r["ok"], str(r))
body = open(CFG).read()
check("comment kept", "# precious comment" in body)
check("delay replaced", "Delay = 9" in body and "Delay = 5\n" not in body)
check("layout0 replaced", 'Layout0 = ["A", "B"]' in body)

before = body
r = req(s, {"cmd": "set_config", "data": {"Keyboard.Delay": "oops"}})
check("bad set rejected", not r["ok"], r.get("error", "")[:60])
check("file untouched after reject", open(CFG).read() == before)

check("validate", req(s, {"cmd": "validate"})["ok"])

# Phase 2: dotted paths into [Action.<name>] tables and the [[WindowClasses]]
# array-of-tables rewrite. The set_config gate must accept both, and the file
# must keep the parts the writers promise not to touch.
r = req(s, {"cmd": "set_config", "data": {
    "Action.Hook1.Timeout": 5.0,
    "WindowClasses": [
        {"Regex": "^smoketest", "MouseClickDrops": False, "Actions": "Actions"},
        {"MouseClickDrops": True, "Actions": "Actions"},
    ],
}})
check("p2 dotted+array set", r["ok"], str(r)[:120])
body2 = open(CFG).read()
check("p2 action header comment kept", "# Run external commands" in body2)
check("p2 dotted key written", "Timeout = 5" in body2)
check("p2 array rewritten", body2.count("[[WindowClasses]]") == 2
      and 'Regex = "^smoketest"' in body2
      and 'Regex = "^(VirtualBox' not in body2 and 'Regex = "^konsole"' not in body2)
check("p2 tail comment kept", "# precious comment" in body2)
check("p2 validate", req(s, {"cmd": "validate"})["ok"])

# Phase 3: the [[AutoReplace]]/[[Hotkeys]] arrays round-trip through the same
# whole-array writer, the parser gate rejects a combo without exactly one
# trigger key, and get_config shows what the disk now holds.
r = req(s, {"cmd": "set_config", "data": {
    "AutoReplace": [
        {"Trigger": "btw", "Replace": "by the way", "Exact": True},
        {"Trigger": "teh", "Replace": "the"},
    ],
    "Hotkeys": [
        {"Keys": ["L_CTRL", "L_ALT", "F1"], "Action": "Layout0"},
    ],
}})
check("p3 arrays set", r["ok"], str(r)[:120])
body3 = open(CFG).read()
def header_count(text, name):
    # Only real table headers count; the shipped config carries commented
    # examples of every section, and a substring count would see those too.
    return sum(1 for ln in text.splitlines() if ln.lstrip().startswith(f"[[{name}]]"))


check("p3 autoreplace written", header_count(body3, "AutoReplace") == 2 and 'Trigger = "btw"' in body3)
check("p3 hotkey written", header_count(body3, "Hotkeys") == 1 and 'Action = "Layout0"' in body3)
check("p3 comment still kept", "# precious comment" in body3)

before3 = body3
r = req(s, {"cmd": "set_config", "data": {"Hotkeys": [{"Keys": ["F1", "F2"], "Action": "Layout0"}]}})
check("p3 two-trigger combo rejected", not r["ok"], r.get("error", "")[:80])
check("p3 file untouched after reject", open(CFG).read() == before3)

r = req(s, {"cmd": "get_config"})
ar = r.get("data", {}).get("AutoReplace")
hk = r.get("data", {}).get("Hotkeys")
check("p3 get_config shows rules", isinstance(ar, list) and len(ar) == 2
      and isinstance(hk, list) and len(hk) == 1, f"{ar} {hk}")
check("p3 validate", req(s, {"cmd": "validate"})["ok"])

s.close()
sys.exit(1 if failures else 0)
