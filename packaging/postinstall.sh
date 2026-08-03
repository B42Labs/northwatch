#!/bin/sh
set -e
if [ "$1" = "configure" ]; then
    if ! getent group northwatch >/dev/null; then
        addgroup --system northwatch
    fi
    if ! getent passwd northwatch >/dev/null; then
        adduser --system --no-create-home --ingroup northwatch \
            --home /var/lib/northwatch --shell /usr/sbin/nologin northwatch
    fi
    # dpkg resolves the archive's group name at unpack time — before this script
    # runs — so on a fresh install /etc/default/northwatch falls back to the
    # numeric gid in the header and lands root:root. Apply the intended group
    # here. Idempotent, so it also repairs an install made by an older package.
    if [ -e /etc/default/northwatch ]; then
        chown root:northwatch /etc/default/northwatch
    fi
    if [ -d /run/systemd/system ]; then
        systemctl daemon-reload
    fi
fi
