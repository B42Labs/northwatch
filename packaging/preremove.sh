#!/bin/sh
set -e
if [ "$1" = "remove" ]; then
    systemctl stop northwatch 2>/dev/null || true
    systemctl disable northwatch 2>/dev/null || true
fi
