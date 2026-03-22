#!/bin/sh
# entrypoint.sh — Set ownership and start clonarr

PUID=${PUID:-99}
PGID=${PGID:-100}

# Fix ownership on /config (includes profiles, custom-cfs, and TRaSH cache)
if [ -d /config ]; then
    chown -R "$PUID:$PGID" /config
fi

exec su-exec "$PUID:$PGID" clonarr
