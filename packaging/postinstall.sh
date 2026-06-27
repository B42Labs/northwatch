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
    if [ -d /run/systemd/system ]; then
        systemctl daemon-reload
    fi
fi
