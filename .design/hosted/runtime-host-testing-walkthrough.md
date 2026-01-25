# Runtime Host API Testing Walkthrough

This guide provides step-by-step instructions for testing the Runtime Host API alongside the Hub API on a Mac with apple-container runtime.

## Prerequisites

- macOS with `container` CLI installed (Apple Virtualization Framework)
- Go 1.21+ installed
- `curl` or similar HTTP client
- `jq` for JSON formatting (optional but recommended)

## 1. Build and Start Both Servers

### Build the Binary

```bash
# From the project root
go build -buildvcs=false -o scion ./cmd/scion
```

### Start Both Hub and Runtime Host APIs

```bash
# Start both servers together
./scion server start --enable-hub --enable-runtime-host
```

You should see output like:
```
2025/01/25 10:00:00 Starting Hub API server on 0.0.0.0:9810
2025/01/25 10:00:00 Database: sqlite (/Users/you/.scion/hub.db)
2025/01/25 10:00:00 Starting Runtime Host API server on 0.0.0.0:9800 (mode: connected)
2025/01/25 10:00:00 Hub API server starting on 0.0.0.0:9810
2025/01/25 10:00:00 Runtime Host API server starting on 0.0.0.0:9800
```

### Alternative: Start Just Runtime Host

If you only want to test the Runtime Host API without the Hub:

```bash
./scion server start --enable-runtime-host
```

### Custom Configuration

```bash
# Custom ports
./scion server start --enable-hub --port 8810 \
  --enable-runtime-host --runtime-host-port 8800

# Read-only mode (reporting only, no agent control)
./scion server start --enable-runtime-host --runtime-host-mode read-only
```

## 2. Test Runtime Host Health Endpoints

### Health Check

```bash
curl -s http://localhost:9800/healthz | jq
```

Expected response:
```json
{
  "status": "healthy",
  "version": "0.1.0",
  "mode": "connected",
  "uptime": "5s",
  "checks": {
    "container": "available"
  }
}
```

The `checks` field shows available runtimes. On Mac with apple-container, you'll see `"container": "available"`.

### Readiness Check

```bash
curl -s http://localhost:9800/readyz | jq
```

Expected response:
```json
{
  "status": "ready"
}
```

### Host Info

```bash
curl -s http://localhost:9800/api/v1/info | jq
```

Expected response:
```json
{
  "hostId": "abc123-...",
  "name": "",
  "version": "0.1.0",
  "mode": "connected",
  "type": "container",
  "capabilities": {
    "webPty": false,
    "sync": true,
    "attach": true,
    "exec": true
  },
  "supportedHarnesses": ["claude", "gemini", "opencode", "generic"],
  "resources": {
    "agentsRunning": 0
  }
}
```

## 3. Agent Management via Runtime Host API

### List Agents

```bash
curl -s http://localhost:9800/api/v1/agents | jq
```

Expected response (empty initially):
```json
{
  "agents": [],
  "totalCount": 0
}
```

### Create an Agent

```bash
curl -s -X POST http://localhost:9800/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{
    "name": "test-agent",
    "config": {
      "template": "claude",
      "task": "Hello, this is a test agent"
    }
  }' | jq
```

Expected response:
```json
{
  "agent": {
    "agentId": "",
    "name": "test-agent",
    "status": "running",
    "containerStatus": "",
    "config": {
      "template": "claude"
    },
    "runtime": {
      "containerId": "container-id-..."
    }
  },
  "created": true
}
```

### Get Agent by ID

```bash
curl -s http://localhost:9800/api/v1/agents/test-agent | jq
```

### Stop an Agent

```bash
curl -s -X POST http://localhost:9800/api/v1/agents/test-agent/stop | jq
```

Expected response:
```json
{
  "status": "accepted",
  "message": "Stop operation accepted"
}
```

### Delete an Agent

```bash
curl -s -X DELETE "http://localhost:9800/api/v1/agents/test-agent?deleteFiles=true"
# Returns 204 No Content on success
```

## 4. Agent Interaction

### Send a Message

Send a message to a running agent's harness (via tmux):

```bash
curl -s -X POST http://localhost:9800/api/v1/agents/test-agent/message \
  -H "Content-Type: application/json" \
  -d '{
    "message": "Please list the files in the current directory",
    "interrupt": false
  }'
```

### Execute a Command

Run a one-off command inside the agent container:

```bash
curl -s -X POST http://localhost:9800/api/v1/agents/test-agent/exec \
  -H "Content-Type: application/json" \
  -d '{
    "command": ["ls", "-la"],
    "timeout": 30
  }' | jq
```

Expected response:
```json
{
  "output": "total 24\ndrwxr-xr-x ...",
  "exitCode": 0
}
```

### Get Logs

```bash
curl -s -X POST http://localhost:9800/api/v1/agents/test-agent/logs
```

Returns plain text logs.

### Get Stats

```bash
curl -s -X POST http://localhost:9800/api/v1/agents/test-agent/stats | jq
```

## 5. Combined Hub + Runtime Host Workflow

This workflow demonstrates how the Hub and Runtime Host APIs work together.

### Step 1: Check Both Servers

```bash
echo "=== Hub Health ==="
curl -s http://localhost:9810/healthz | jq

echo -e "\n=== Runtime Host Health ==="
curl -s http://localhost:9800/healthz | jq
```

### Step 2: Register a Grove with Host Info

Register a grove with the Hub, including this runtime host's information:

```bash
curl -s -X POST http://localhost:9810/api/v1/groves/register \
  -H "Content-Type: application/json" \
  -d '{
    "gitRemote": "https://github.com/myorg/myproject.git",
    "name": "My Project",
    "mode": "connected",
    "host": {
      "name": "My MacBook",
      "version": "0.1.0",
      "runtimes": [
        {"type": "container", "available": true}
      ],
      "capabilities": {
        "webPty": false,
        "sync": true,
        "attach": true
      },
      "supportedHarnesses": ["claude", "gemini", "opencode", "generic"]
    }
  }' | jq
```

This creates:
- A grove in the Hub
- A runtime host record
- A grove contributor relationship
- Returns a host token for authentication

### Step 3: Create Agent via Hub

```bash
# Use the grove ID from step 2
GROVE_ID="your-grove-id"

curl -s -X POST http://localhost:9810/api/v1/agents \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Feature Agent\",
    \"groveId\": \"$GROVE_ID\",
    \"template\": \"claude\"
  }" | jq
```

### Step 4: List Agents on Runtime Host

```bash
curl -s http://localhost:9800/api/v1/agents | jq
```

### Step 5: Verify Agent in Hub

```bash
curl -s "http://localhost:9810/api/v1/agents?groveId=$GROVE_ID" | jq
```

## 6. Read-Only Mode Testing

Start the Runtime Host in read-only mode:

```bash
./scion server start --enable-runtime-host --runtime-host-mode read-only
```

### Verify Read-Only Mode

```bash
curl -s http://localhost:9800/healthz | jq '.mode'
# Returns: "read-only"
```

### List Works

```bash
curl -s http://localhost:9800/api/v1/agents | jq
# Returns agent list
```

### Create Blocked

```bash
curl -s -X POST http://localhost:9800/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{"name": "blocked-agent"}' | jq
```

Expected response (405):
```json
{
  "error": {
    "code": "operation_not_allowed",
    "message": "Operation not allowed in read-only mode"
  }
}
```

## 7. Error Handling

### Agent Not Found

```bash
curl -s http://localhost:9800/api/v1/agents/nonexistent | jq
```

Expected response (404):
```json
{
  "error": {
    "code": "agent_not_found",
    "message": "Agent not found"
  }
}
```

### Validation Error

```bash
curl -s -X POST http://localhost:9800/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{}' | jq
```

Expected response (400):
```json
{
  "error": {
    "code": "validation_error",
    "message": "name is required"
  }
}
```

### Invalid JSON

```bash
curl -s -X POST http://localhost:9800/api/v1/agents \
  -H "Content-Type: application/json" \
  -d '{invalid}' | jq
```

Expected response (400):
```json
{
  "error": {
    "code": "invalid_request",
    "message": "Invalid request body: ..."
  }
}
```

## 8. Full Workflow Script

Save this as `test-runtime-host.sh`:

```bash
#!/bin/bash
set -e

HUB_URL="http://localhost:9810"
HOST_URL="http://localhost:9800"

echo "=== 1. Health Checks ==="
echo "Hub:"
curl -s $HUB_URL/healthz | jq '.status'
echo "Runtime Host:"
curl -s $HOST_URL/healthz | jq '{status, mode, checks}'

echo -e "\n=== 2. Runtime Host Info ==="
curl -s $HOST_URL/api/v1/info | jq '{type, mode, capabilities}'

echo -e "\n=== 3. Register Grove with Host ==="
GROVE_RESPONSE=$(curl -s -X POST $HUB_URL/api/v1/groves/register \
  -H "Content-Type: application/json" \
  -d '{
    "gitRemote": "https://github.com/test/demo-project",
    "name": "Demo Project",
    "mode": "connected",
    "host": {
      "name": "Test Mac",
      "version": "0.1.0",
      "runtimes": [{"type": "container", "available": true}],
      "capabilities": {"sync": true, "attach": true}
    }
  }')
echo $GROVE_RESPONSE | jq
GROVE_ID=$(echo $GROVE_RESPONSE | jq -r '.grove.id')
echo "Grove ID: $GROVE_ID"

echo -e "\n=== 4. List Agents (should be empty) ==="
curl -s $HOST_URL/api/v1/agents | jq

echo -e "\n=== 5. Create Agent via Hub ==="
AGENT_RESPONSE=$(curl -s -X POST $HUB_URL/api/v1/agents \
  -H "Content-Type: application/json" \
  -d "{
    \"name\": \"Demo Agent\",
    \"groveId\": \"$GROVE_ID\",
    \"template\": \"claude\"
  }")
echo $AGENT_RESPONSE | jq
AGENT_ID=$(echo $AGENT_RESPONSE | jq -r '.agent.id')
echo "Agent ID: $AGENT_ID"

echo -e "\n=== 6. List Agents in Hub ==="
curl -s "$HUB_URL/api/v1/agents?groveId=$GROVE_ID" | jq '.agents[] | {name, status}'

echo -e "\n=== 7. List Runtime Hosts ==="
curl -s $HUB_URL/api/v1/runtime-hosts | jq '.hosts[] | {name, type, status}'

echo -e "\n=== 8. Final Health Stats ==="
curl -s $HUB_URL/healthz | jq '.stats'

echo -e "\n=== Done! ==="
```

Run it:
```bash
chmod +x test-runtime-host.sh
./test-runtime-host.sh
```

## 9. Cleanup

### Stop the Server

Press `Ctrl+C` to gracefully shutdown both servers.

### Reset Database

```bash
rm ~/.scion/hub.db
```

### Clean Up Test Agents

If you created real agents with containers:

```bash
# List scion containers
container list | grep scion

# Stop and remove
container stop <container-name>
container rm <container-name>
```

## Troubleshooting

### Port Already in Use

```bash
# Find process using port 9800
lsof -i :9800

# Use different ports
./scion server start --enable-runtime-host --runtime-host-port 9801
```

### Container Runtime Not Found

If you see `"runtime": "unavailable"` in health checks:

```bash
# Verify container CLI is installed
which container

# Check container runtime status
container version
```

### No Agents Listed

The Runtime Host API lists agents that are:
1. Actually running as containers with `scion.agent=true` label
2. Have agent directories in known grove paths

If you created an agent via the Hub but haven't started it on this host, it won't appear in the Runtime Host agent list.

### Permission Issues

```bash
# Ensure scion directory exists and is writable
mkdir -p ~/.scion
chmod 755 ~/.scion
```

## API Reference Summary

### Runtime Host API (Port 9800)

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/healthz` | GET | Liveness check |
| `/readyz` | GET | Readiness check |
| `/api/v1/info` | GET | Host information |
| `/api/v1/agents` | GET | List agents |
| `/api/v1/agents` | POST | Create agent |
| `/api/v1/agents/{id}` | GET | Get agent details |
| `/api/v1/agents/{id}` | DELETE | Delete agent |
| `/api/v1/agents/{id}/start` | POST | Start agent |
| `/api/v1/agents/{id}/stop` | POST | Stop agent |
| `/api/v1/agents/{id}/restart` | POST | Restart agent |
| `/api/v1/agents/{id}/message` | POST | Send message |
| `/api/v1/agents/{id}/exec` | POST | Execute command |
| `/api/v1/agents/{id}/logs` | POST | Get logs |
| `/api/v1/agents/{id}/stats` | POST | Get stats |

### Hub API (Port 9810)

See `hub-api-testing-walkthrough.md` for full Hub API documentation.
