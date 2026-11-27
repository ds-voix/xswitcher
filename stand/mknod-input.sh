#!/bin/bash
# Docker --privileged does not expose kernel input devices created after the container
# started; recreate them from sysfs, which is visible. Idempotent.
set -u

mkdir -p /dev/input

# /dev/uinput: misc device, static 10:223 when the module is loaded on the host kernel.
if [ ! -e /dev/uinput ] && [ -e /sys/class/misc/uinput ]; then
    mknod /dev/uinput c 10 223 2>/dev/null || true
fi

for d in /sys/class/input/event*; do
    [ -e "$d/dev" ] || continue
    name=$(basename "$d")
    IFS=: read -r maj min < "$d/dev"
    [ -e "/dev/input/$name" ] && continue
    mknod "/dev/input/$name" c "$maj" "$min" 2>/dev/null || true
done

ls -l /dev/uinput /dev/input/ 2>&1 | sed 's/^/  /'
