# Security policy

## What this program is

xswitcher is a keylogger by design. It reads every keyboard event from `/dev/input`, and it injects
events through a `uinput` virtual keyboard. Layout switching and `RetypeWord` are only possible because
of that. So the honest threat model is not "can an attacker read keystrokes through xswitcher" -- the
program already does that for the user who runs it. The questions worth treating as vulnerabilities are
listed below.

## Supported versions

| Version | Supported |
|---|---|
| `main` of this repository | yes |
| release assets published by `.github/workflows/release.yml` | yes |
| the tag `v1.0` before that pipeline ran | no -- binaries built before the gates existed |
| upstream `ds-voix/xswitcher` | only what its own maintainer supports |

A published asset is verifiable rather than trusted: `xswitcher --version` prints
`xswitcher <tag> (<commit>)`, and `ci.yml` fails if that line does not match the commit it was built
from. `checksums.txt` accompanies every release.

## What counts as a vulnerability

1. **Anything that turns typed text into code execution for a person who cannot already edit the
   config.** `[Exec]` actions run shell commands as the user who runs the daemon, and `[[Actions]]`
   rules are matched against typed text, so whoever can write `/etc/xswitcher/xswitcher.conf` can run
   commands as that user. The package installs the config `0644` (measured with `dpkg-deb -c`), which is
   deliberate for a per-machine tune-up file -- if your threat model includes other local users, tighten
   the mode or run the daemon under a dedicated account. A report that demonstrates this path from an
   attacker who cannot write the config is in scope.
2. **A privilege regression in the packaging.** The shipped binaries are `0755 root:root` with no setuid
   bit (measured inside the built `.deb`), and the post-install script changes nothing: it neither edits
   `/etc/group` nor `chown`s anything. A build, package or workflow change that reintroduces setuid,
   setgid, `CAP_SET*`, or a root daemon that keeps running with no user session is a vulnerability, not a
   convenience.
3. **Escape from the X display / input boundary.** Reading another user's keystrokes through a socket,
   device or path that the config was not pointed at; attaching a device the `[ScanDevices] Bypass`
   pattern excludes (the rig checks this as `F1 a bypassed device is closed again`); or a second reader on
   the same device after re-creation (checked as the single-reader phase).
4. **IPC exposure.** The daemon listens on a Unix socket -- `XSWITCHER_SOCK` when the environment names
   one, otherwise `DefaultSocketPath` = `/run/xswitcher/xswitcher.sock`, and `Start()` chmods it `0660`
   (`internal/ipc/server.go`). Anything that lets a different user talk to that socket -- `status`, config
   writes, reload -- is in scope, because `status` carries the collected word buffer and config writes
   reach item 1.
5. **A malicious or mismatched dependency.** `govulncheck ./...` is a mandatory gate in `ci.yml`; a report
   that a reachable vulnerability is *not* covered by it (wrong toolchain pin, a code path the scan does
   not trace) is worth filing.

## What does not count

- "xswitcher records my keystrokes, including passwords." Yes, that is the feature. Disable or reconfigure
  it per the README; that is not a security defect.
- Anything that needs already-root access on the machine.
- The Wayland bypass (`[Wayland] BypassX = true`) switching to the wrong layout because the desktop uses a
  different shortcut. This is documented, measured and a configuration mismatch: `stand/run.sh` asserts the
  emitted `EV_KEY` sequence is exactly the configured `Layout1`, and the README states that the daemon
  neither observes the desktop's layout nor warns when the shortcut does not match.
- Timing side channels in the retype burst.

## How to report

Open a private vulnerability report against `ds-voix/xswitcher` through GitHub's security tab of that
repository; if private reporting is not enabled there, open a draft security advisory against the same
repository instead. Please do not open a public issue first.

Along with the description, the useful artifacts here are the daemon log, the config in use, and which
build: the output of `xswitcher --version`. The daemon writes to stdout and stderr and has no syslog
backend, so under the packaged user unit the log is read with `journalctl --user -u xswitcher`; `-d`
raises it to debug level, and `DUMPLOG=1` in the stand streams the whole log.
