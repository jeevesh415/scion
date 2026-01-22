#!/bin/bash
# List scion agents with optional JSON output
# Usage: list-agents.sh [--json] [--all]

JSON_OUTPUT=""
ALL_FLAG=""

while [[ $# -gt 0 ]]; do
    case $1 in
        --json)
            JSON_OUTPUT="--format json"
            shift
            ;;
        --all|-a)
            ALL_FLAG="--all"
            shift
            ;;
        *)
            shift
            ;;
    esac
done

scion list $ALL_FLAG $JSON_OUTPUT
