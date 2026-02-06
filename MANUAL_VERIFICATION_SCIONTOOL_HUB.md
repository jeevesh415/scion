# Manual Verification: Agent-to-Hub Status Updates

This document outlines the steps to manually verify that a Hub-provisioned agent correctly sends status updates and heartbeats to the Hub using `sciontool`.

## Prerequisites

1.  **Scion Hub** running (default port 9810).
2.  **Runtime Broker** running and registered with the Hub.
3.  **CLI** configured to use the Hub (`scion hub login`).

## Steps

### 1. Provision a Hosted Agent

Start an agent in hosted mode. This will ensure the Hub generates a JWT and the Runtime Broker injects the necessary environment variables.

```bash
# Create a grove if one doesn't exist
scion grove create my-project

# Start an agent
scion start --harness gemini --grove my-project --name test-agent
```

### 2. Verify Environment Variables

Attach to the running agent and verify the Hub-related environment variables are set.

```bash
# Attach to the agent
scion attach test-agent

# Inside the agent container:
env | grep SCION_HUB
env | grep SCION_AGENT_ID
```

You should see:
*   `SCION_HUB_URL`: The URL of the Hub API.
*   `SCION_HUB_TOKEN`: A JWT starting with `eyJ...`.
*   `SCION_AGENT_ID`: The UUID of the agent.

### 3. Send Status Updates via sciontool

Use `sciontool` within the agent container to send status updates.

```bash
# Inside the agent container:

# 1. Signal waiting for input
sciontool status ask_user "What is the next task?"

# 2. Signal task completion
sciontool status task_completed "Fixed the logging bug"
```

### 4. Verify Hub State

From your host machine, query the Hub API to verify the agent's state has been updated.

```bash
# Get agent info from Hub
scion hub agent get test-agent
```

**Check the following in the output:**

*   After `ask_user`:
    *   `status` should be `idle` (or the mapped Hub status).
    *   `message` should be `"What is the next task?"`.
    *   `lastSeen` should be recent.
*   After `task_completed`:
    *   `status` should reflect completion (e.g., `idle` with a completion flag or `completed`).
    *   `taskSummary` should contain `"Fixed the logging bug"`.
    *   `lastSeen` should be recent.

### 5. Verify Heartbeats

`sciontool` can also be used to send heartbeats manually (though it's usually handled by the supervisor).

```bash
# Inside the agent container:
# (Currently sciontool status doesn't have a direct 'heartbeat' subcommand, 
# but it's called internally by other commands or can be added)
```

Verify that `lastSeen` updates on the Hub after any `sciontool status` command.

## Troubleshooting

*   **401 Unauthorized**: Check if `SCION_HUB_TOKEN` is valid and not expired.
*   **403 Forbidden**: Verify the token has the `agent:status:update` scope and matches the `SCION_AGENT_ID`.
*   **404 Not Found**: Verify `SCION_AGENT_ID` matches an existing agent on the Hub.
*   **Connection Refused**: Verify `SCION_HUB_URL` is reachable from within the container.
