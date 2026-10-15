#!/bin/sh
# Fix ownership on mounted volumes (may be root-owned from previous versions),
# then drop to a non-root user to run the application.
#
# PUID / PGID (optional): run as that user and group ID instead of the
# built-in isley user, and give the data, uploads and logs folders to it.
# Unraid's template sets 99 / 100 (nobody:users), the owner Unraid expects
# for appdata. Without them, nothing changes: the isley user (100:101).

if [ "$(id -u)" = "0" ]; then
    owner="isley:isley"
    if [ -n "$PUID" ] || [ -n "$PGID" ]; then
        case "${PUID:-x}${PGID:-x}" in
            *[!0-9]*)
                echo "PUID and PGID must both be numbers (got PUID='$PUID' PGID='$PGID'); running as isley" >&2
                ;;
            *)
                owner="$PUID:$PGID"
                ;;
        esac
    fi
    mkdir -p /app/data /app/uploads /app/logs
    chown -R "$owner" /app/data /app/uploads /app/logs 2>/dev/null || true
    if [ "$owner" = "isley:isley" ]; then
        exec su-exec isley /app/isley "$@"
    fi
    exec su-exec "$owner" /app/isley "$@"
else
    # Already running as non-root (docker run --user ...)
    exec /app/isley "$@"
fi
