#!/usr/bin/env bash
# === stand/run-gui-stand.sh ===
# Host wrapper: bring the GUI stand container up and run the checks inside it.
#
#   bash stand/run-gui-stand.sh
#
# Everything it does is reversible: the container is named xswitcher-gui-stand and the only
# host state it touches is the kernel module loading done by stand/setup-host.sh.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
REPO="$(cd "$HERE/.." && pwd)"
CONTAINER=xswitcher-gui-stand

command -v docker >/dev/null || { echo "FATAL: docker not in PATH"; exit 9; }
docker info >/dev/null 2>&1 || { echo "FATAL: the Docker daemon is not running"; exit 9; }

# uinput and evdev are modules in the Docker Desktop VM kernel and nothing autoloads them,
# so this has to be repeated after every VM restart.
echo "--- kernel modules ---"
bash "$HERE/setup-host.sh"

echo "--- container ---"
COMMIT="$(git -C "$REPO" rev-parse --short HEAD)"
VERSION="$(git -C "$REPO" describe --always --tags --dirty)"
echo "stamp: Commit=$COMMIT Version=$VERSION"

GUISTAND_COMMIT="$COMMIT" GUISTAND_VERSION="$VERSION" \
    docker compose -f "$HERE/docker-compose.gui.yml" up -d --build

echo "--- checks ---"
# The path is written as //w on purpose. Git Bash rewrites an argument that starts with /w into the
# Windows form W:/w before handing it to the native docker.exe, so the container then tries to run
# "bash W:/w/stand/gui-stand.sh" and dies with "No such file or directory" (measured: exit 127, last
# line "bash: W:/stand/gui-stand.sh: No such file or directory"). A leading double slash is left alone
# by the conversion and means the same directory inside Linux.
docker exec -e GUISTAND_COMMIT="$COMMIT" -e GUISTAND_VERSION="$VERSION" \
    "$CONTAINER" bash //w/stand/gui-stand.sh

echo ""
echo "The GUI of the stand is on http://localhost:6081/vnc.html (noVNC, passwordless)."
echo "Stop it with:  docker compose -f stand/docker-compose.gui.yml down"
