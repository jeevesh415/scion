#!/bin/bash
# Start a scion agent with a task
# Usage: start-agent.sh <name> <task> [--type template] [--attach]

if [ $# -lt 2 ]; then
    echo "Usage: start-agent.sh <name> <task> [--type template] [--attach]"
    exit 1
fi

NAME="$1"
shift
TASK="$1"
shift

# Pass remaining args to scion
scion start "$NAME" "$TASK" "$@"
