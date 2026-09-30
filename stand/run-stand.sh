#!/bin/bash
# Run the whole stand from the Windows host: load the kernel modules, then the container.
#   bash stand/run-stand.sh
set -u
HERE=$(cd "$(dirname "$0")" && pwd)
REPO=$(cd "$HERE/.." && pwd)
DOCKER="${DOCKER:-/f/Docker/resources/bin/docker}"
WS=$(cygpath -w "$REPO")

bash "$HERE/setup-host.sh" || { echo "host modules unavailable"; exit 9; }
mkdir -p "$REPO/.gomodcache"

MSYS_NO_PATHCONV=1 "$DOCKER" run --rm --privileged \
    -e DUMPLOG="${DUMPLOG:-}" \
    -v "$WS:/w" \
    -v "$WS/.gomodcache:/root/go/pkg/mod" \
    xswitcher-stand bash /w/stand/run.sh
