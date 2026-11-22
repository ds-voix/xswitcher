# X language switcher v1.0 ("xswitcher")
Xswitcher is the fully customizable low-level keyboard helper for X.Org-based Linux desktops.  
It consists of two main parts: low-level keylogger and the virtual keyboard.  
On each key up/down event, xswitcher checks the last events chain against configured regex-based patterns.  
If one matches, xswitcher triggers the associated action. Action chains can be joined from simple actions.  
Look at TOML-based explained config file "xswitcher.conf" for further details.

## Retype
One of the most powerful actions of xswitcher is to retype the collected event buffer
after switching the language. Because of it's low-level nature, it may know nothing about layout details.  
So, xswitcher does nothing strange, in comparison with punto switcher on windows.  
It just sends a number of BackSpaces to wipe the last input, then calls language switcher
(internal, via X API, or external, via call of the configured program), an then retypes the same
keyboard events by means of virtual keyboard.

## Gnome is the pain
Note that the users of actual gnome-based shells can't use the XOrg embedded language switcher.
Due to the gnome built-in bug, X language API is broken by gnome since 2011.
However, there's the workaround. Look under the "gnome3" subfolder for customized config file and the simple helper "switch.gnome".
The helper header contains the brief instruction on how to deal with the https://github.com/lyokha/g3kb-switch.git

## Precompiled xswitcher for the beginners
The release assets carry the prebuilt binaries: one portable `tar.gz` per architecture, a `.deb`
and an `.rpm` (amd64 and arm64). The packaged daemon is installed to `/usr/bin/xswitcher`, the GUI to
`/usr/bin/xswitcher-gui`, the config to `/etc/xswitcher/xswitcher.conf` and a **user** systemd unit to
`/usr/lib/systemd/user/xswitcher.service`:

    sudo dpkg -i xswitcher_1.1.0_amd64.deb      # or: sudo rpm -Uvh xswitcher-1.1.0-1.x86_64.rpm
    sudo usermod -aG input "$USER"              # then re-login
    systemctl --user enable --now xswitcher

Being the low-level keylogger, xswitcher needs access to "/dev/input/" devices. It gets them through the
"input" group, not through ownership or privileges: the packaged binaries are `0755 root:root` with no setuid
bit, and the post-install script prints the two commands above instead of editing `/etc/group` behind your
back. Measured on the snapshot build: `dpkg-deb -c` shows `-rwxr-xr-x root/root` for both binaries.

Also note that, being an honest keylogger by nature, xswitcher can be of course configured to do something strange.  
E.g. you can configure it to email all the input to e.g. "fbi@usa.gov". Nobody bans.

## CLI
There are a number of helpful CLI flags:

    xswitcher -h
    Usage of :
      -c, --conf string   Non-default config location (default "/etc/xswitcher/xswitcher.conf")
      -d, --debug         Debug log level
      -t, --test          Only output all key events to STDERR. No actions.
      -v, --verbose       Increase log level to NOTICE

Use "-v" or even "-d" to debug when You implements any config changes.  
Use "-t" to collect the keyboard events string to construct Your custom actions.

## Respawn
This version of xswitcher do the scan of input devices only once (on it's launch).  
But there is the internal "respawn" action in case You connects new keyboard/mouse.  
By default, the long press of "Scroll lock" triggers "respawn". (Respawn does the complete self-restart).

## Adding a language

Translations live under `internal/l10n/locales/`. One JSON file per
language, named by the BCP-47 base tag (`en.json`, `ru.json`, `de.json`).
Region variants collapse to their base (`pt_BR` and `pt` both resolve to
`pt`), so keep one file per language unless you actually need divergent
translations for two regions.

Every catalogue MUST define two meta-keys that name the language itself:

    {
      "language.name": "<endonym shown in the picker>",
      "language.english": "<same language named in English>"
    }

The picker displays `<name> (<english>)` when they differ and just
`<name>` when they are identical. A catalogue without these two keys still
loads but shows its file code instead of a human-readable label.

Every other key is the string passed to `l10n.X("key", "English literal")`
in the source. The English value in `en.json` MUST be byte-identical to the
literal at the call site; `go test ./internal/l10n` enforces this parity
and also checks that every non-English catalogue carries the same set of
keys and the same `{{.Placeholder}}` names as English. Run it after any
catalogue edit -- it is the drift guard.

Plurals use `l10n.XN("key", "other-form", count, ...)`. The JSON entry
uses go-i18n plural forms (`one`, `few`, `many`, `other`); Russian gets
`one/few/many/other` automatically via `x/text/plural`.

Locale resolution order:

1. `[GUI] Language` in the config (set by the GUI picker).
2. `XSWITCHER_LANG` environment variable (handy for systemd units:
   `Environment=XSWITCHER_LANG=ru`).
3. OS locale (`LC_ALL` / `LC_MESSAGES` / `LANG`, with `LANGUAGE`
   overriding when the slot is not `C`/`POSIX`).
4. English fallback.

An unknown or empty code at any step is skipped; an unknown explicit
choice in `[GUI] Language` is rejected so a typo cannot silently pin
the UI to the wrong file.

## Themes

The GUI ships three built-in choices -- `system`, `dark`, `light` -- and any
number of your own. A theme is a JSON file under the user config directory:

    ~/.config/xswitcher/themes/<name>.json

(on Windows: `%APPDATA%\xswitcher\themes`; override the whole directory with
the `XSWITCHER_THEMES_DIR` environment variable.) The picker in the About tab
lists the built-ins plus every `*.json` basename found there, applies the
choice immediately, and persists it as `[GUI] Theme` through the daemon, so it
survives a restart and is read before the first widget is built.

A file may set any subset of two blocks; everything it omits comes from the
Fyne palette of the chosen `variant`:

    {
      "variant": "dark",
      "name": "Neon",
      "colors": {
        "background": "#101418ff",
        "primary": "#00e5b0ff",
        "foreground": "#e6edf3ff"
      },
      "sizes": {
        "text": 14,
        "padding": 8,
        "inputRadius": 6
      }
    }

`variant` is `"dark"` or `"light"` (anything else is treated as dark) and picks
the fallback palette. `colors` keys are Fyne theme color names (28 in total, for
example `background`, `foreground`, `primary`, `button`, `menuBackground`,
`inputBackground`, `hyperlink`, `error`), and values are hex with a leading
`#`: `#rrggbb` or `#rrggbbaa`.
`sizes` keys are Fyne theme size names (19 in total, for example `text`,
`padding`, `innerPadding`, `lineSpacing`, `inputRadius`, `scrollBar`) with
floating-point values in pixels.

A malformed file, a hex value the loader cannot parse, or a theme name that
tries to leave the themes directory (`..`, `/`, `\`) is refused: the picker
shows why and keeps the previous theme.

## How to build.
You must install (libX11-devel | libx11-dev) package for X bindings.  
* The package name may, of course, be different in Your distro.  
X calls are made through cgo, so a C compiler ("gcc" or "clang") must be present too.  
And, of course, you must have the working go environment.  
The module pins the toolchain exactly: `go 1.26.0` plus `toolchain go1.26.11`. That is not a style
choice: `govulncheck` traces into the standard library, so what survives depends on the patch release,
not on this code. Measured on the CI runner on 2026-11-22 at the previous pin: go1.26.10 left one
reachable standard-library advisory, GO-2026-6859 in `net/http`, which is fixed in go1.26.11. The scan
stays a blocking gate, so the next drift in the advisory database fails the pipeline instead of making
this paragraph quietly wrong.
The stand images follow the same pin (`golang:1.26-trixie`), so a local run, the container and CI measure
the same compiler.  
Build from the repository root:

    go build -o xswitcher ./src/

Nothing else is required. "go.mod" resolves every dependency by itself, no "go get" round is needed.  
The virtual keyboard library is not fetched from the network either:

    replace github.com/micmonay/keybd_event => ./src/keybd_event/

points it at the sources kept under "src/keybd_event", where my "keybd_linux_export.go" already lives.  
(Unfortunately my pull request https://github.com/micmonay/keybd_event/pull/32 still stays unaccepted.)  
"embeddedConfig", "exec" and "scancodes" are imported through the module name "xswitcher", so they must not
be moved under "src/" as the earlier version of this instruction told You to do.

There is exactly one module in the repository: the "stand" helpers are part of it too, so the clipboard holder
the rig needs builds from the root as well.

    go build -o /tmp/cliphold ./stand/cliphold/

A second "go.mod" under "stand/" silently hides that package from the root module ("main module (xswitcher)
does not contain package xswitcher/stand/cliphold"), which is how the rig lost a check without going red.
Build output is never a source: "/bin/" is ignored, the rig writes its own "/w/bin/xswitcher-gui" at run time.

The resulting binary is not static: it links libX11 (and libxcb) dynamically, as any X desktop provides them.  
To strip it, as I do for the release assets, add the linker flags:

    go build -o xswitcher -ldflags "-s -w" ./src/ && chmod 755 xswitcher

Put the config to "/etc/xswitcher/xswitcher.conf" (or just use embedded one)
and install xswitcher executable under "/usr/local/bin/" or where You prefer.

Make it executable ("chmod 755", no setuid bit: the daemon reads "/dev/input" as a member of the "input"
group), configure the autostart inside Your X GUI and enjoy.

## How to verify.
xswitcher is a keylogger on top of the kernel input layer, so an honest check needs "/dev/input" and "uinput",  
not a mocked keyboard. The "stand" folder holds the end-to-end rig that does exactly this inside one privileged  
Linux container. It builds the daemon, runs it, creates a *second* virtual keyboard through uinput as the source  
device, injects keys into it and then asserts on the EV_KEY stream xswitcher emits on its own keyboard:

* "go build", "go vet" and "go test ./..." pass;
* "HELLO" + Pause wipes 5 symbols, sends the layout shortcut and retypes the word;
* a focus loss in the middle of a word does not wipe the collected word (the buffers survive "None" focus);
* a lone LeftCtrl tap only cycles the layout, with no BackSpaces at all.

Run it from the repository root:

    bash stand/run-stand.sh

Both stand images install their system packages from one file, `stand/apt-deps.txt`, which is also what
`.github/workflows/ci.yml` reads -- so a missing build header is missing in exactly one place. `stand/Dockerfile`
used to carry its own shorter list, and that is what broke check B3 (`go test ./...`) of the rig: Fyne pulls
glfw, `glfw3native.h` includes `<X11/extensions/Xrandr.h>`, and that header only comes from `libxrandr-dev`.

It needs docker (or podman) with "--privileged" and the "uinput" + "evdev" modules loaded on the host kernel.  
On Windows 11 with Docker Desktop those modules are built but not autoloaded in the "docker-desktop" VM, so  
"stand/setup-host.sh" loads them; it has to be re-run after every WSL VM restart. No desktop session, no  
full distro installation and no second machine are required. Set "DUMPLOG=1" to get the whole daemon log.

The same rig has a GUI half, and it is the half that watches a real key press instead of a config file:

    bash stand/run-gui-stand.sh

Both wrappers are meant to be run from Git Bash on Windows as written, with no extra environment
variables: `run-stand.sh` sets `MSYS_NO_PATHCONV` for its own `docker run`, and `run-gui-stand.sh` asks for
the in-container path as `//w/...`, because Git Bash would otherwise hand the native docker binary a
`W:/w/...` argument that does not exist inside the container.

It runs 20 checks (G1..G8) and refuses to go green if the count shrinks, builds the daemon and the Fyne GUI
from the checkout it is pointed at, and reads the stamp back off the binary it produced:

    xswitcher --version          # -> "xswitcher v1.1.0 (2d66d12)", one line, no root and no config needed

That line is what a release asset is matched against, so an unstamped build stays visible as
"xswitcher dev (unknown)" instead of pretending to be a release.

The GUI stand used to be a second, older layer as well: `stand/run-gui.sh`, `stand/run-gui-docker.sh`,
`stand/runtime.sh`, `stand/runtime-gui.yml` and two READMEs describing an Alpine image, `Xvfb :99` and a
`Win+Space` shortcut. None of it is what runs now -- the image is `golang:1.26-trixie`, the display is a
private slot chosen by the script, and the shortcut comes from the config -- so the layer was dropped
(6 files, 452 lines) instead of being kept as documentation that contradicts the rig. Nothing outside those
six files referenced them.

What a container does and does not prove. Xvfb serves a multi-group keymap only when started with
"-noreset": every observer here (setxkbmap, xgroup, the eye) is a one-shot client, and without that flag
the loaded map is restored the moment the last client leaves -- measured as "groups 1" without the flag and
"groups 2" with it. With the flag the rig really moves the live server: check G4 of `stand/gui-stand.sh`
asserts both that the server answers "locked 1" and that the daemon's log carries
"XkbLockGroup(1) answered 1". What no container here proves is a compositor: the Wayland bypass is measured
on the emitted EV_KEY stream (checks W1/W2 of `stand/run.sh`), never inside a real Wayland session.

## KDE@Wayland: shit happens
Wayland cancels all the X groundwork for multi-language systems.
While Gnome forces it's own dbus-based API, KDE does nothing at all.
X-layer is just a stub built into the current KDE environment. It doesn't contain any real data.
So, the only common things are now the kernel-based "/dev/input" and virtual keyboard.
Window classes are gone together with the language API.

I wrote the "[Wayland]" crutch. It selects language via keyboard shortcuts.
For example, one can configure "Win+1" to select language #0 and "Win+2" to select language #1.
Then, in "xswitcher.conf":

    [Wayland]
     BypassX = true # Switch layouts via pre-configured keyboard shortcuts
     Layout0 = ["L_META:1", "1:1", "1:0", "L_META:0"] # Win+1
     Layout1 = ["L_META:1", "2:1", "2:0", "L_META:0"] # Win+2
     Delay = 50

* It takes about 50 ms to complete switching the keyboard layout in my KDE, so I set 50 as the default value.
* Up to 4 language shortcuts "Layout0"…"Layout3".

What the crutch actually emits is measured, not assumed. With the shipped `BypassX = true` and a word typed
on the wrong layout, the daemon writes exactly this shape to its own uinput keyboard (recorded 23.10 with
`stand/sniff.py`, asserted as W1/W2 in `stand/run.sh`):

    125:1  3:1  3:0  125:0        # L_META down, KEY_2 down/up, L_META up -- [Wayland] Layout1
    14:1 14:0  x5                 # one BackSpace per collected character
    25:1 25:0 33:1 33:0 ...       # the word replayed with the SAME evdev codes

Two consequences are worth knowing before filing a bug. The daemon does not re-encode the letters into
Cyrillic: it hands the desktop environment the shortcut and retypes the physical keys, so the translation is
done by whatever that shortcut selects. And nothing here observes the result -- if the real desktop uses a
different binding (GNOME's default is Super+Space, not Super+1/Super+2), the wipe and the replay still happen
while the layout never moves. Checks W1 and W2 are what keeps this honest: W1 counts `XkbLockGroup` in the
log and expects zero, W2 matches the replayed sequence against the configured `Layout1`.

## How to release.
A release is a tag: pushing `v1.1.0` runs `.github/workflows/release.yml`, which installs the build
dependencies from `stand/apt-deps.txt` plus the derived arm64 cross set, verifies the stamp against the tag,
and hands the tree to goreleaser (`.goreleaser.yaml`). The result is one `tar.gz` per architecture, a `.deb`
and an `.rpm` per architecture, and `checksums.txt`, published as assets of that tag.

The stamp is read from the ref that triggered the run, not from `git describe`. `actions/checkout` does not
fetch tags unless `fetch-tags: true` is asked for, and in a clone without tags
`git describe --tags --abbrev=0` answers `fatal: No names found, cannot describe anything.` -- measured in a
`--no-tags` clone of this repository, where `git tag -l` returns nothing. That is how the first release run of
`v1.1.0` built a binary whose version string was empty, so the step now asserts two things separately: that a
tag push carries a `v*` ref, and that the binary answers with exactly the stamp that was compiled in.

A job that runs in a container has to name its shell and has to make git trust the checkout; neither is
inherited. Both were measured, not guessed. The dispatched run of 2026-10-26 executed the stamp step as
`sh -e {0}` and it stopped on `COMMIT="${GITHUB_SHA::7}"` with `Bad substitution` and exit code 2 -- a
bash-only substitution, and the image does ship `/usr/bin/bash`, so the default is not something to rely on;
the same script body under `bash --noprofile --norc -eo pipefail` exits 0. Then git: the steps run as root
while `actions/checkout` leaves the tree owned by the runner's uid, and `go build` answers
`error obtaining VCS status: exit status 128` (measured exit 1). One
`git config --global --add safe.directory "$GITHUB_WORKSPACE"` later the same build exits 0 and the binary
prints `xswitcher main (53f155a)`. So `defaults.run.shell: bash` is written in all three workflow files, and
the two container jobs have the trust step. `-buildvcs=false` is not the fix: the stamp and the changelog are
supposed to come from git.

The manual `workflow_dispatch` run is the dry run, and it cannot publish: `main` carries no tag, and
goreleaser answers `git doesn't contain any tags - either add a tag or use --snapshot` with exit code 1. The
dispatched run therefore adds `--snapshot`, measured to exit 0 on a tagless tree and to stamp the binary
`xswitcher v0.0.0 (<sha>)` -- a version nobody can mistake for a release -- while a `v*` tag push keeps the
plain `release --clean`. That tag path was measured end to end in the same image: `tar.gz`, `.deb` and `.rpm`
for amd64 and arm64 (six packages) plus `checksums.txt`, exit code 0 in 3m16s.

Nothing is trusted from the build log: the asset answers for itself.

    xswitcher --version            # "xswitcher v1.1.0 (a1b2c3d)" -- tag and commit, one line

The gates that have to be green before a tag exists live in `.github/workflows/ci.yml`: `gofmt`,
`go build`, `go vet`, `go test -count=1`, the stamp comparison and `govulncheck`. `.github/workflows/stand.yml`
is a separate, manually dispatched question -- whether a hosted runner gives the rig `--privileged` and a
usable `/dev/uinput`; until it is answered it stays off the push path so it cannot redden a branch.

## Reporting a vulnerability.
See `SECURITY.md`. In short: xswitcher is a keylogger by design, so "it records keystrokes" is not a
vulnerability; what is in scope is a privilege regression in the packaging (no setuid, `0755 root:root` --
both measured inside the built `.deb`), a path from typed text to code execution for someone who cannot edit
the config, an escape across the socket, and a reachable dependency finding that `govulncheck` misses.

## Packaging
I don't have enough time to maintain any distro package (rpm, dpkg, etc.).  
But it seems to be a good deed. So, everybody who is ready to do this job is welcome.

The two lines above are the author's own words and are left untouched; the offer in them is
what this slice answers, and it answers it for Linux only. `.github/workflows/release.yml` runs
goreleaser on a `v*` tag and publishes a portable `tar.gz`, a `.deb` and an `.rpm` for amd64 and
arm64, with `checksums.txt`. The recipe lives in `.goreleaser.yaml`, the systemd unit in
`packaging/systemd/user/xswitcher.service` and the post-install notice in
`packaging/scripts/post-install.sh`. The whole set was built and inspected locally with
`goreleaser release --snapshot --clean`: 6 assets, both binaries inside each package at
`0755 root:root`, `rpm -qp -R` reporting `glibc, libX11, libxkbcommon, mesa-libGL`, and the
packaged daemon answering `xswitcher v1.0 (2fc2441)` to `--version`.

## Attribution

This section documents who produced the work, not who owns the project. The audit, fixes and
regression guards in these commits were produced as a stacked PR against upstream
`ds-voix/xswitcher`; the commits carry per-issue verdicts (#14 fixed here, #12 guarded and
attributed to upstream `5fe6e3a`, #5 defect + feature, #13/#1 no code changes). Changes by BaHooo.

- Upstream: <https://github.com/ds-voix/xswitcher>

## Support

The project is free and stays free. If the audit and the fixes in these commits are worth
a coffee, the channels are listed in [`.github/wallets.md`](.github/wallets.md):
DonationAlerts plus three crypto wallets (ETH, TON, BTC). Nothing in the project depends on
it - no feature, no fix, no answer to an issue sits behind a paywall, and the Sponsor
button keeps pointing at the upstream account.

## Conclusion
Please, don't be shy to report the bug if something in this README looks unclear.
