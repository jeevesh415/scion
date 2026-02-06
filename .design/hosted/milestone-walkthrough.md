# Hosted Architecture End-to-End Milestone Walkthrough

**Created:** 2026-02-02
**Updated:** 2026-02-03
**Status:** ✅ Milestone Complete
**Goal:** Enable end-to-end manual integration testing of the hosted architecture

---

## 1. Target Milestone Scenarios

The following end-to-end user scenarios define the milestone:

1. **Authenticate the CLI** with the Hub
2. **Use a locally defined template** to start an agent (exercising remote template infrastructure - push to cloud storage and register with Hub)
3. **Attach to the agent** and interact with it over tmux
4. **Synchronize the workspace** back to the local machine
5. **Stop the agent**
6. **Remove the agent**

These scenarios should work with Hub server and Runtime Broker running on different machines (or emulated via separate processes on the same machine).

---

## 2. Current Implementation Status

### 2.1 What's Fully Implemented

| Component | Status | Key Files |
|-----------|--------|-----------|
| **CLI Authentication** | ✅ Complete | `cmd/hub_auth.go` |
| - OAuth browser-based login | ✅ | `scion hub auth login` |
| - Dev auth fallback | ✅ | |
| - Credential storage | ✅ | `pkg/credentials/` |
| **Template Management** | ✅ Complete | `cmd/templates.go` |
| - `scion template sync` (create/update in Hub) | ✅ | |
| - `scion template push` (upload files to GCS) | ✅ | |
| - `scion template pull` (download from Hub) | ✅ | |
| - GCS storage via rclone | ✅ | `pkg/gcp/storage.go` |
| - Signed URL generation | ✅ | `pkg/hub/template_handlers.go` |
| **Hub Registration** | ✅ Complete | `cmd/hub.go` |
| - `scion hub register` | ✅ | |
| - `scion hub deregister` | ✅ | |
| - `scion hub status` | ✅ | |
| - HMAC-based broker authentication | ✅ | `pkg/hub/hostauth.go`, `pkg/runtimebroker/hostauth.go` |
| - Bidirectional HMAC (Hub→Broker signing) | ✅ | `pkg/hub/brokerclient.go` |
| - Secret rotation endpoint | ✅ | `POST /api/v1/brokers/{id}/rotate-secret` |
| - Nonce cache (replay prevention) | ✅ | Enabled by default |
| **Agent Lifecycle (Hub Mode)** | ✅ Complete | `cmd/create.go`, `cmd/start.go`, `cmd/stop.go`, `cmd/delete.go` |
| - Create via Hub | ✅ | |
| - Start via Hub | ✅ | |
| - Stop via Hub | ✅ | |
| - Delete via Hub | ✅ | |
| **HTTP Dispatcher** | ✅ Complete | `pkg/hub/httpdispatcher.go` |
| - Dispatch to remote Runtime Brokers via HTTP | ✅ | |
| - Authenticated dispatch (HMAC-signed) | ✅ | `pkg/hub/brokerclient.go` |
| **Runtime Broker API** | ✅ Complete | `pkg/runtimebroker/` |
| - Agent lifecycle endpoints | ✅ | |
| - Template cache/hydration | ✅ | `pkg/templatecache/` |
| - Heartbeat to Hub | ✅ | `pkg/runtimebroker/heartbeat.go` |
| - Strict auth mode (configurable) | ✅ | `BrokerAuthStrictMode` config |
| **Observability** | ✅ Complete | |
| - Audit logging | ✅ | `pkg/hub/audit.go` |
| - Broker auth metrics | ✅ | `pkg/hub/metrics.go`, `/metrics` endpoint |

### 2.2 Recently Implemented

| Component | Status | Key Files |
|-----------|--------|-----------|
| **PTY/Attach via Hub** | ✅ Complete | `cmd/attach.go`, `pkg/wsclient/pty.go` |
| - WebSocket PTY relay | ✅ | `pkg/hub/pty_handlers.go` |
| - PTY stream multiplexing | ✅ | `pkg/hub/controlchannel.go` |
| **WebSocket Control Channel** | ✅ Complete | `pkg/hub/controlchannel.go` |
| - Hub-initiated commands | ✅ | HTTP tunneling via WebSocket |
| - NAT/firewall traversal | ✅ | Broker-initiated connection |

### 2.3 All Scenarios Complete ✅

All blocking scenarios have been implemented. Workspace sync was the final piece.

| Component | Status | Key Files |
|-----------|--------|-----------|
| **Workspace Sync (Hosted)** | ✅ Complete | `cmd/sync.go`, `pkg/hub/workspace_handlers.go` |
| - Sync workspace files to/from remote broker | ✅ | `pkg/runtimebroker/workspace_handlers.go` |
| - rclone integration for workspace | ✅ | `pkg/gcp/storage.go` |
| - Signed URL pattern (like templates) | ✅ | `pkg/hubclient/workspace.go` |
| - Incremental sync via content hashing | ✅ | `pkg/transfer/` |

### 2.4 Implementation Notes

**Workspace Sync:**
- `cmd/sync.go` extended with hosted mode detection via `CheckHubAvailability()`
- Uses the signed URL pattern (same as templates) for direct CLI ↔ GCS transfer
- Hub coordinates sync, Runtime Broker uploads/applies via rclone
- Shared `pkg/transfer` package provides common file transfer functionality
- Incremental sync via SHA-256 content hashing (skip unchanged files)

---

## 3. Scenario-by-Scenario Analysis

### Scenario 1: Authenticate the CLI ✅

**Status:** Fully implemented

**Commands:**
```bash
# Set Hub endpoint
export SCION_HUB_ENDPOINT=http://hub.example.com:9000

# Authenticate via browser OAuth
scion hub auth login

# Verify authentication
scion hub status
```

**What Happens:**
1. CLI opens browser for OAuth flow
2. User authenticates with OAuth provider
3. Access token stored in `~/.config/scion/credentials/<endpoint-hash>/credentials.json`
4. Subsequent commands use stored token

**No Implementation Work Required.**

---

### Scenario 2: Use Local Template to Start Agent ⚠️

**Status:** Partially implemented - **requires configuration and testing**

**Commands:**
```bash
# Push local template to Hub (uploads to GCS, registers in Hub)
scion template sync custom-claude \
  --from .scion/templates/claude \
  --scope grove \
  --harness claude

# Start agent using the template
scion start my-agent --type custom-claude "Fix the login bug"
```

**What Works:**
- Template sync/push/pull CLI commands
- GCS storage via rclone
- Signed URL generation
- Template resolution in agent creation
- HTTP dispatch to Runtime Broker
- Template hydration on Runtime Broker

**Configuration Required:**

1. **GCS Bucket Setup:**
   - Create bucket: `gs://scion-hub-<env>/`
   - Configure in Hub settings

2. **Service Account Credentials:**
   - Service account with `storage.objects.create`, `storage.objects.get`
   - `iam.serviceAccounts.signBlob` for signed URLs
   - For dev: `gcloud auth application-default login --impersonate-service-account=<sa>`

3. **Hub Storage Configuration:**
   ```yaml
   hub:
     storage:
       provider: "gcs"
       bucket: "scion-hub-dev"
   ```

4. **Runtime Broker Template Cache:**
   ```yaml
   runtimeBroker:
     templateCache:
       path: "~/.scion/cache/templates"
       maxSize: "100MB"
   ```

**Gap: Runtime Broker Endpoint Discovery**

When Hub dispatches to Runtime Broker, it needs the broker endpoint URL. Currently:
- Broker registers with Hub and provides endpoint URL
- Hub stores endpoint in database
- Dispatcher looks up endpoint for dispatch

**Resolved:** Runtime Brokers behind NAT use the WebSocket control channel:
- Broker initiates WebSocket connection to Hub at `/api/v1/runtime-brokers/connect`
- Hub tunnels HTTP requests through the control channel
- No external endpoint URL needed for NAT-ed brokers
- See `runtimebroker-websocket.md` Section 9 for implementation details

---

### Scenario 3: Attach and Interact via tmux ✅

**Status:** Fully implemented

**Commands:**
```bash
scion attach my-agent
# Connects via WebSocket through Hub to Runtime Broker
```

**What Happens:**
1. CLI calls Hub to get agent details and verify running status
2. CLI establishes WebSocket connection to Hub at `/api/v1/agents/{id}/pty`
3. Hub opens PTY stream to Runtime Broker via control channel
4. Runtime Broker executes `docker exec -i {container} tmux attach-session -t scion`
5. Bidirectional I/O is relayed: CLI ↔ Hub ↔ Runtime Broker ↔ Container

**Implementation Files:**
- `pkg/hub/pty_handlers.go` - Hub PTY WebSocket endpoint
- `pkg/hub/controlchannel.go` - Stream multiplexing to hosts
- `pkg/runtimebroker/pty_handlers.go` - Docker exec and PTY handling
- `pkg/runtimebroker/controlchannel.go` - Control channel client
- `pkg/wsclient/pty.go` - CLI WebSocket client
- `cmd/attach.go` - Updated to use WebSocket when Hub enabled

**Features:**
- Terminal raw mode for proper character handling
- SIGWINCH resize event propagation
- Bearer token authentication
- Graceful disconnect handling

**No Implementation Work Required.**

---

### Scenario 4: Synchronize Workspace to Local Machine ✅

**Status:** Fully implemented

**CLI Commands:**
```bash
# Sync workspace from remote agent to local
scion sync from my-agent

# Sync local changes to remote agent
scion sync to my-agent

# Preview what would be synced (dry-run)
scion sync from my-agent --dry-run

# Exclude patterns from sync
scion sync to my-agent --exclude "*.log" --exclude "tmp/**"
```

**What Happens (Sync FROM):**
1. CLI calls Hub API: `POST /api/v1/agents/{id}/workspace/sync-from`
2. Hub tunnels request to Runtime Broker via control channel
3. Runtime Broker uploads workspace to GCS using rclone
4. Hub generates signed download URLs for each file
5. CLI downloads files directly from GCS (incremental - skips unchanged)

**What Happens (Sync TO):**
1. CLI scans local workspace and computes file hashes
2. CLI calls Hub API: `POST /api/v1/agents/{id}/workspace/sync-to` with file list
3. Hub checks which files already exist in storage (by hash)
4. Hub returns signed upload URLs for new/changed files only
5. CLI uploads files directly to GCS
6. CLI calls Hub API: `POST /api/v1/agents/{id}/workspace/sync-to/finalize`
7. Hub tunnels apply request to Runtime Broker
8. Runtime Broker downloads from GCS and applies to container workspace

**Key Implementation Files:**
- `cmd/sync.go` - CLI sync command with hosted mode
- `pkg/hub/workspace_handlers.go` - Hub workspace endpoints
- `pkg/runtimebroker/workspace_handlers.go` - Runtime Broker handlers
- `pkg/hubclient/workspace.go` - Hub client workspace service
- `pkg/transfer/` - Shared file transfer package

**Design Decisions:**
- On-demand sync only (explicit command, not automatic)
- Last-write-wins for conflict handling
- Incremental sync via SHA-256 content hashing
- 15-minute signed URL expiry (same as templates)
- No file size limits (GCS handles large files natively)

**No Further Implementation Work Required.**

---

### Scenario 5: Stop the Agent ✅

**Status:** Fully implemented

**Commands:**
```bash
scion stop my-agent
```

**What Happens:**
1. CLI calls Hub API: `POST /api/v1/agents/{id}/stop`
2. Hub dispatches to Runtime Broker via HTTP
3. Runtime Broker stops the agent container

**No Implementation Work Required.**

---

### Scenario 6: Remove the Agent ✅

**Status:** Fully implemented

**Commands:**
```bash
scion delete my-agent
# Or
scion stop my-agent --rm
```

**What Happens:**
1. CLI calls Hub API: `DELETE /api/v1/agents/{id}`
2. Hub dispatches to Runtime Broker via HTTP
3. Runtime Broker stops container, removes files, optionally removes git branch
4. Hub removes agent record from database

**No Implementation Work Required.**

---

## 4. Implementation Priority

### Phase 1: Configuration & Testing (Day 1)
**Goal:** Verify existing functionality works end-to-end

1. Set up GCS bucket with proper permissions
2. Configure Hub and Runtime Broker settings
3. Test template push/pull workflow
4. Test agent create/start/stop/delete workflow
5. Document any issues discovered

### Phase 2: Workspace Sync (Days 2-3)
**Goal:** Enable syncing workspace files

1. Add `workspace` prefix to Hub storage
2. Implement sync trigger on Runtime Broker (on-demand initially)
3. Add Hub endpoint for workspace sync metadata
4. Update `scion sync` command for hosted mode
5. Test with rclone

### Phase 3: PTY Attach (Days 4-6)
**Goal:** Enable interactive agent sessions

1. Implement WebSocket PTY endpoint on Hub
2. Implement PTY attachment on Runtime Broker
3. Update CLI attach command to use WebSocket
4. Handle terminal resize, disconnect, reconnect
5. Test interactive sessions

### Phase 4: Polish & Documentation (Day 7)
**Goal:** Complete milestone

1. Error handling and edge cases
2. User-facing documentation
3. Integration test script
4. Update status.md

---

## 5. Open Questions for Decision

### Q1: Workspace Sync Direction

When syncing workspaces, what's the primary direction?

**Options:**
- **A. Download-only:** Workspace is authoritative on remote, sync pulls to local
- **B. Bidirectional:** Changes can be made locally and pushed to remote
- **C. On-demand both:** Explicit `sync to` and `sync from` commands (current local behavior)

**Recommendation:** Option C - explicit commands, matching current local behavior

### Q2: Sync Storage Location

Where should workspace snapshots be stored?

**Options:**
- **A. Same bucket as templates:** `gs://scion-hub-{env}/workspaces/{groveId}/{agentId}/`
- **B. Separate bucket per grove:** `gs://{groveId}-workspaces/`
- **C. User-configurable:** Allow different storage backends

**Recommendation:** Option A - simpler, one bucket to manage

### Q3: Attach Authentication

How should CLI authenticate WebSocket connections for PTY?

**Options:**
- **A. Query parameter token:** `ws://hub/agents/{id}/pty?token=<bearer>`
- **B. Ticket-based:** Request short-lived ticket first, use in WebSocket
- **C. Cookie-based:** If Hub shares session with web frontend

**Recommendation:** Option B - more secure, aligns with design doc

### Q4: Runtime Broker Endpoint Registration

How does Runtime Broker specify its externally-reachable endpoint?

**Options:**
- **A. Explicit flag:** `scion server start --endpoint http://myhost:9800`
- **B. Auto-detect:** Determine from network interfaces
- **C. Registration response:** Hub tells broker its observed IP

**Recommendation:** Option A - explicit is more reliable, especially for dev

---

## 6. Test Setup

### Local Emulation (Single Machine)

Run Hub and Runtime Broker as separate processes:

```bash
# Terminal 1: Start Hub
scion server start --enable-hub --hub-port 9000

# Terminal 2: Start Runtime Broker (different port)
scion server start --enable-runtime-broker --broker-port 9800 \
  --hub-endpoint http://localhost:9000 \
  --endpoint http://localhost:9800

# Terminal 3: CLI operations
export SCION_HUB_ENDPOINT=http://localhost:9000
scion hub auth login
scion hub register
scion template sync my-template --from .scion/templates/claude --harness claude
scion start my-agent --type my-template "Hello world"
scion attach my-agent  # Now works via WebSocket
scion sync from my-agent  # Will fail until implemented
scion stop my-agent
scion delete my-agent
```

### Distributed Setup

Same commands but with actual different machines:
- Hub: `hub.example.com:9000`
- Runtime Broker: `broker.example.com:9800`
- CLI: Developer laptop

---

## 7. Success Criteria

The milestone is complete when:

1. ✅ CLI can authenticate with Hub
2. ✅ Local template can be pushed to Hub (GCS)
3. ✅ Agent can be started on remote Runtime Broker using pushed template
4. ✅ CLI can attach to remote agent and interact via tmux
5. ✅ Workspace can be synced from remote agent to local machine
6. ✅ Agent can be stopped via CLI
7. ✅ Agent can be removed via CLI

**7 of 7 scenarios complete.** 🎉 Milestone achieved!

All scenarios work with Hub and Runtime Broker running as separate processes.

---

## 8. Related Documentation

| Document | Relevance |
|----------|-----------|
| [status.md](status.md) | Overall implementation status |
| [hosted-architecture.md](hosted-architecture.md) | System design |
| [hosted-templates.md](hosted-templates.md) | Template management design |
| [runtimebroker-websocket.md](runtimebroker-websocket.md) | WebSocket/PTY design |
| [hub-api.md](hub-api.md) | Hub API specification |
| [runtime-broker-api.md](runtime-broker-api.md) | Runtime Broker API specification |
| [hub-api-testing-walkthrough.md](hub-api-testing-walkthrough.md) | Hub API testing guide |
| [runtime-broker-testing-walkthrough.md](runtime-broker-testing-walkthrough.md) | Runtime Broker testing guide |
