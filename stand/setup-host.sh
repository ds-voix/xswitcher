#!/bin/bash
# Load the kernel modules the stand needs into the Docker Desktop WSL2 kernel.
# Both are built as modules (=m) and nothing autoloads them in that minimal VM,
# so this has to be repeated after every VM restart (Docker Desktop quit, wsl --shutdown).
set -u
W="wsl.exe -d docker-desktop -u root --"
MSYS_NO_PATHCONV=1 $W modprobe uinput
MSYS_NO_PATHCONV=1 $W modprobe evdev
for m in uinput evdev; do
    if MSYS_NO_PATHCONV=1 $W ls -d /sys/module/$m >/dev/null 2>&1; then
        printf '%-10s loaded\n' "$m"
    else
        printf '%-10s MISSING\n' "$m"; exit 1
    fi
done
