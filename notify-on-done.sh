#!/bin/bash
# Watch for a background session to complete, then notify
SESSION_ID="$1"
MSG="$2"
while true; do
    # Check if process is still running
    if ! ps -p "$3" > /dev/null 2>&1; then
        openclaw system event --text "$MSG" --mode now 2>/dev/null
        exit 0
    fi
    sleep 5
done
