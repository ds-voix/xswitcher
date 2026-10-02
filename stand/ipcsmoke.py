#!/usr/bin/env python3
# Live IPC smoke against a running daemon (start it with XSWITCHER_SOCK and
# CONFIG pointing at /tmp/GuiSmoke.conf first). Covers Phase 0 of
# GUI-PLAN-PUNTO.md: ping/status/get_config/set_config/validate over the real
# unix socket, comment preservation, and rejection of an invalid mod.
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

s.close()
sys.exit(1 if failures else 0)
