# GUI stand: checking a layout switch with real keystrokes

`stand/gui-stand.sh` answers one question that no unit test can answer: when a key is actually
pressed, does xswitcher switch the layout of the live server and retype the word, and does the
result reach a real X client?

Run it with the wrapper:

```bash
bash stand/run-gui-stand.sh          # container + checks, one command
```

or, if the container is already up:

```bash
docker exec -e GUISTAND_COMMIT=$(git rev-parse --short HEAD) \
            -e GUISTAND_VERSION=$(git describe --always --tags --dirty) \
    xswitcher-gui-stand bash /w/stand/gui-stand.sh
```

The final line carries the number of checks behind it:

```
checks run                                               20 / GUI-STAND: ALL GREEN
```

## Why the stand is built the way it is

A browser-driven test cannot observe this program. Xvfb serves a machine with "no display
hardware and no physical input devices" (`Xvfb(1)`), so:

* keys typed in the noVNC window go XTEST -> Xvfb, and the daemon, which reads **evdev**, never
  sees them;
* the daemon's output goes to its own uinput keyboard, which Xvfb equally ignores, so the GUI
  window receives nothing.

Measured both directions: three taps written straight into the source node produced **0** events
in an X client (check G2), and the same daemon run with the bridge produced **10**.

So the stand has three parts around the real binaries:

| part | file | role |
|---|---|---|
| hand | `stand/source_key.py` | a uinput keyboard the daemon reads, i.e. the human |
| bridge | `stand/bridge.c` | replays the daemon's own keyboard (`keybd interface`) into X through XTEST |
| eye | `stand/echo.c` | prints the X keycode, its evdev equivalent, the keysym name and the live Xkb group |

The bridge only shifts codes: `X keycode = evdev code + 8` (measured on eight codes, and
documented as "kernel keycode + 8" by Peter Hutterer, <http://who-t.blogspot.com/2021/>). It
never touches the layout, because the daemon owns that through `XkbLockGroup`, and the same code
already decodes to `a` in group 1 and to `Cyrillic_ef` in group 2.

`BypassX` must be `false` for this stand. With the shipped `true` the daemon replays `Meta+digit`
instead of calling `XkbLockGroup`, and the layout of the live server does not move at all -- the
stand would then be measuring a code path Xvfb ignores.

## What the 20 checks assert

| check | assertion |
|---|---|
| G1 x8 | every evdev code used by the bridge maps to `code + 8`, verified by the keysym name |
| G2 | without the bridge an X client sees nothing of the source device |
| G3 | the bridge attached to the daemon's own keyboard |
| G4 x2 | the live server moved to group 1, and the log shows `XkbLockGroup(1) answered 1` |
| G5 x3 | the word was wiped (5 BackSpace), came back as 5 Cyrillic keysyms, in the exact mapping `p->ze f->a y->en d->ve f->a` |
| G6 | exactly 10 KeyPress reached the client: no loop, no duplicates |
| G8 x2 | the daemon's IPC reports `word_buffer":"pfydf"` and `word_group":1` |
| G7 x2 | the GUI maps a window on the stand's display and a screenshot was captured |

The typed word is `pfydf`, which is `занва` under the Russian layout -- an arbitrary sequence of
keys, deliberately not a real word, so that a mapping mistake cannot look like a typo.

## Why the stand uses Xvfb and not Xorg

Spike of 23.10 (#101): the same container was given a real X server -- `xserver-xorg-core` +
`xserver-xorg-video-dummy`, `Xorg :98 -config <dummy layout>`, 1280x1024, `vendor release number 12101016`.
It starts, and it is unusable for these checks. Xorg attaches **zero** input devices here: no `udevd` runs in
the container (`/run/udev` does not exist, `udevadm` is present but idle), so hotplug gives nothing even with
`AutoAddDevices=true` and a live `/dev/input/event0 = stand-source-keyboard` already created through uinput:
the log of that run carries zero lines matching "configuring input device".
What is left is Xorg's default core keyboard, and on it `setxkbmap -layout us,ru,de` reports success while
`setxkbmap -query` answers `layout: us` -- one group, no way to observe a switch.

Xvfb is the opposite case, and that is why the rig keeps it: its core keyboard has XKB, and with `-noreset`
a multi-group map survives between the one-shot observers, which is what makes G4 ("the live server moved to
group 1" / "the daemon moved it through XkbLockGroup") assertable at all. The consequence for anyone reading
this as a compatibility claim: the rig proves the protocol and the code paths, not the behaviour of a desktop
X server, and a real Debian desktop has not been measured.

## What this stand does not check

* It does not check what the user sees in the browser. noVNC is there to look at the window while
  the stand runs (`http://localhost:6081/vnc.html`), not to feed input; input from the browser
  cannot reach the daemon, for the reason above.
* It does not check Wayland. `[Wayland] BypassX = true` is the upstream default and is switched off here:
  on this branch the daemon replays `Meta+digit`, so Xvfb would never move and the eye would measure
  nothing. The bypass branch is measured where it can be seen -- on the emitted EV_KEY stream, as checks
  W1/W2 of `stand/run.sh`: W1 expects zero `XkbLockGroup` calls, W2 expects the configured `Layout1`
  (`125:1 3:1 3:0 125:0`), recorded 23.10 over a 24-event stream.
* It does not decode UTF-8 in the eye. The image ships `libX11-dev` without `X11/Xutf8.h`, so the
  eye prints the keysym name (`Cyrillic_ef`) instead of the character.

## Files

```
stand/Dockerfile.gui        the container: golang:1.26-trixie + Xvfb/x11vnc/noVNC/websockify
stand/docker-compose.gui.yml  build context = repository root, sources mounted at /w
stand/run-gui-stand.sh      host wrapper: modules, container, checks
stand/gui-stand.sh          the checks themselves
stand/echo.c stand/bridge.c stand/xgroup.c    eye, bridge, layout observer
stand/source_key.py stand/mknod-input.sh      hand, device nodes
stand/setup-host.sh         uinput/evdev in the Docker Desktop VM kernel
```
