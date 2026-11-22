#!/bin/sh
# post-install of the xswitcher package (deb and rpm share this script).
#
# It deliberately changes nothing: no setuid bit, no chown of the binaries, no user added to a group.
# Membership in "input" is a decision for the administrator, and a package script that edits /etc/group
# on its own is exactly the kind of surprise upstream issue #15 asks to remove. The script only tells
# the operator what the daemon needs to work.
set -u

if [ "${1:-}" = "abort-upgrade" ] || [ "${1:-}" = "abort-remove" ] || [ "${1:-}" = "abort-deconfigure" ]; then
    exit 0
fi

if ! getent group input >/dev/null 2>&1; then
    echo "xswitcher: there is no 'input' group on this system; create it before enabling the service." >&2
fi

cat >&2 <<'MSG'
xswitcher is installed as a user service and reads /dev/input directly, so the user running
it must belong to the "input" group (no setuid, no root daemon):

    sudo usermod -aG input "$USER"    # then re-login

    systemctl --user enable --now xswitcher
    systemctl --user status xswitcher

Configuration: /etc/xswitcher/xswitcher.conf
MSG

exit 0
