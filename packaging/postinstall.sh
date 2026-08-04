#!/bin/sh
set -e
if [ "$1" = "configure" ]; then
    # --quiet keeps dpkg's output clean: without it adduser reports "Not
    # creating home directory" for the --no-create-home it was just asked for.
    # Warnings and errors still get through.
    if ! getent group northwatch >/dev/null; then
        addgroup --system --quiet northwatch
    fi
    if ! getent passwd northwatch >/dev/null; then
        adduser --system --quiet --no-create-home --ingroup northwatch \
            --home /var/lib/northwatch --shell /usr/sbin/nologin northwatch
    fi
    # dpkg resolves the archive's user/group names at unpack time — before this
    # script runs — so on a fresh install the shipped paths fall back to the
    # numeric ids in the header and land root:root. Apply the intended ownership
    # here. Idempotent, so it also repairs an install made by an older package
    # (which left /var/lib/northwatch to the unit's StateDirectory=).
    if [ -e /etc/default/northwatch ]; then
        chown root:northwatch /etc/default/northwatch
    fi
    if [ -d /var/lib/northwatch ]; then
        chown northwatch:northwatch /var/lib/northwatch
        chmod 0750 /var/lib/northwatch
    fi
    if [ -d /run/systemd/system ]; then
        systemctl daemon-reload
    fi
fi
