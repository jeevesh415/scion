#!/bin/bash
set -e

# hack/run-claude.sh - Launch the main scion-claude container for testing
# This script is intended for local testing of the Claude image with persistent 
# home directory and a specific workspace.

# Full image path from pkg/config/embeds/default_settings.json
IMAGE=${1:-us-central1-docker.pkg.dev/ptone-misc/public-docker/scion-claude:latest}
WORKSPACE="/Users/ptone/src/claude/testing-workspace"

echo "=== Launching Claude container ==="
echo "Image:   $IMAGE"
echo "Mount:   $HOME -> /home/node"
echo "Workdir: $WORKSPACE"

# Ensure the workspace directory exists on the host
mkdir -p "$WORKSPACE"

# Build the command array for consistent echoing and execution
RUN_CMD=(
  container run -it
  --rm
  -v "${HOME}:/home/node"
  -v "${WORKSPACE}:${WORKSPACE}"
  -w "${WORKSPACE}"
  "$IMAGE"
  claude
)

echo "Executing: ${RUN_CMD[*]}"

# Use 'container' (Apple Virtualization Framework CLI) instead of 'docker'
"${RUN_CMD[@]}"

