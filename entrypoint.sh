#!/bin/sh
# GoPulley container entrypoint
#
# Handles two execution scenarios:
#
# 1) Running as root (UID 0):
#    Standard Docker usage OR Podman rootless without keep-id
#    (in rootless Podman the host user maps to container UID 0).
#    Fix /data ownership, then drop to the gopulley user.
#
# 2) Running as non-root:
#    Podman rootless with userns_mode: keep-id (compose.podman.yml).
#    The host user UID is preserved inside the container and already
#    owns the data directory — exec directly.
#
set -e

if [ "$(id -u)" = "0" ]; then
    chown -R gopulley:gopulley /data
    exec su-exec gopulley /app/gopulley "$@"
fi

exec /app/gopulley "$@"
