#!/bin/sh
# entrypoint.sh — Set ownership and start clonarr

PUID=${PUID:-99}
PGID=${PGID:-100}

# Fix ownership on volumes (/config recursively for profiles subdir, /data top-level only)
if [ -d /config ]; then
    chown -R "$PUID:$PGID" /config
fi
if [ -d /data ]; then
    chown "$PUID:$PGID" /data
fi

exec su-exec "$PUID:$PGID" clonarr
