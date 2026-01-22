#!/bin/bash
# Send a message to an agent
# Usage: send-message.sh <agent-name> <message> [--interrupt]

if [ $# -lt 2 ]; then
    echo "Usage: send-message.sh <agent-name> <message> [--interrupt]"
    exit 1
fi

AGENT="$1"
shift
MESSAGE="$1"
shift

scion message "$AGENT" "$MESSAGE" "$@"
