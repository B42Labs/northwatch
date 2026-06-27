#!/bin/sh
set -e
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    systemctl daemon-reload || true
fi
# On purge, drop the persisted state (SQLite history DB). The northwatch system
# user/group is intentionally retained; an orphaned system account is harmless.
if [ "$1" = "purge" ]; then
    rm -rf /var/lib/northwatch
fi
