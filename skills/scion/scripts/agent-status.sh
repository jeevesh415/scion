#!/bin/bash
# Get status of a specific agent or all agents
# Usage: agent-status.sh [agent-name]
# Returns JSON with agent information

if [ -n "$1" ]; then
    # Get specific agent status
    scion list --format json | jq --arg name "$1" '.[] | select(.name == $name or .Name == $name)'
else
    # Get all agents as JSON
    scion list --format json
fi
