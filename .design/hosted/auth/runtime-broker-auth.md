# Runtime Broker Authentication Design

## Status
**Phase 4 Implemented** (Production Hardening)

## 1. Overview

This document specifies how Runtime Brokers authenticate with the Scion Hub using HMAC-based request signing. Runtime Brokers are compute nodes that execute agents on behalf of the Hub, and require a secure bidirectional authentication mechanism distinct from user/agent authentication.

### 1.1 Relationship to Server Auth

The Hub's unified authentication middleware (see [server-auth-design.md](server-auth-design.md)) handles user, agent, and API key authentication. Runtime Broker authentication is a **separate pathway** that:

- Uses HMAC signatures rather than bearer tokens
- Authenticates infrastructure components, not users or agents
- Enables bidirectional trust (Hub can push commands to brokers)
- Operates at the broker level, not the request/session level

| Authentication Type | Token/Mechanism | Direction | Purpose |
|---------------------|-----------------|-----------|---------|
| User (Web/CLI) | JWT Bearer | Client → Hub | User API access |
| Agent (sciontool) | JWT Bearer | Agent → Hub | Agent status updates |
| **Runtime Broker** | **HMAC Signature** | **Bidirectional** | **Broker ↔ Hub trust** |

### 1.2 Goals

1. **Mutual Authentication** - Both Hub and Runtime Broker can verify each other's identity
2. **Replay Prevention** - Timestamped, nonce-protected requests prevent replay attacks
3. **Secure Bootstrap** - One-time secret exchange with user authorization
4. **Minimal Exposure** - Shared secret never transmitted after initial registration
5. **Offline Verification** - No external service calls required for signature validation

### 1.3 Non-Goals

- Token-based authentication (use JWT for agents/users instead)
- Session management (each request is independently verified)
- Rate limiting (handled by separate middleware)

---

## 2. Architecture

### 2.1 Components

```
┌─────────────────┐                    ┌─────────────────┐
│   Scion Hub     │◄──── HTTPS ────────│  Runtime Broker   │
│                 │      (HMAC)        │                 │
│  ┌───────────┐  │                    │  ┌───────────┐  │
│  │ Secret    │  │                    │  │ Secret    │  │
│  │ Store     │  │                    │  │ Store     │  │
│  │(per broker)│  │                    │  │ (local)   │  │
│  └───────────┘  │                    │  └───────────┘  │
│                 │                    │                 │
│  ┌───────────┐  │                    │  ┌───────────┐  │
│  │ HMAC      │  │                    │  │ HMAC      │  │
│  │ Verifier  │  │                    │  │ Signer    │  │
│  └───────────┘  │                    │  └───────────┘  │
└─────────────────┘                    └─────────────────┘
```

### 2.2 Header Specification

All HMAC-authenticated requests include these headers:

| Header | Format | Description |
|--------|--------|-------------|
| `X-Scion-Broker-ID` | UUID or slug | Unique identifier for the Runtime Broker |
| `X-Scion-Timestamp` | RFC 3339 | Request timestamp (e.g., `2025-01-30T12:00:00Z`) |
| `X-Scion-Nonce` | Base64 (16 bytes) | Random nonce for replay prevention |
| `X-Scion-Signature` | Base64 (32 bytes) | HMAC-SHA256 signature |

---

## 3. Phase 1: Initial Registration (Bootstrap)

Before HMAC authentication can work, both parties need a shared secret. This is the only phase where a credential is transmitted.

### 3.1 Registration Flow

```
┌──────────────┐        ┌──────────────┐        ┌─────────────┐
│  Admin User  │        │ Runtime Broker │        │     Hub     │
│   (CLI/Web)  │        │              │        │             │
└──────────────┘        └──────────────┘        └─────────────┘
       │                       │                       │
       │ POST /api/v1/brokers    │                       │
       │ (with user token)     │                       │
       │──────────────────────────────────────────────►│
       │                       │                       │
       │                       │   { brokerId, joinToken, expiry }
       │◄──────────────────────────────────────────────│
       │                       │                       │
       │ Provide joinToken     │                       │
       │──────────────────────►│                       │
       │                       │                       │
       │                       │ POST /api/v1/brokers/join
       │                       │ { brokerId, joinToken, publicInfo }
       │                       │──────────────────────►│
       │                       │                       │
       │                       │   { secretKey, hubEndpoint }
       │                       │◄──────────────────────│
       │                       │                       │
       │                       │ [Store secret locally]│
       │                       │                       │
```

### 3.2 Registration Steps

1. **Broker Creation (User-Initiated)**
   - An authorized user (admin or broker manager) calls the Hub to register a new broker
   - Hub generates a unique `brokerId` and a short-lived `joinToken`
   - User receives the join token to provide to the broker operator

2. **Broker Join (Broker-Initiated)**
   - The Runtime Broker sends its `joinToken` to the Hub's bootstrap endpoint
   - This must occur over HTTPS
   - Broker includes public metadata (hostname, profiles, version)

3. **Secret Exchange**
   - Hub generates a high-entropy secret key ($K_s$) using `crypto/rand`
   - Hub stores `hash($K_s$)` associated with the `brokerId`
   - Hub returns $K_s$ to the Runtime Broker (one-time transmission)
   - Runtime Broker stores $K_s$ in local secure storage

### 3.3 Simplified CLI Registration

In the common case where a user is logged into the Runtime Broker machine and already authenticated with the Hub, the registration flow can be streamlined via the CLI:

```bash
# User runs this on the Runtime Broker machine
scion hub brokers join --name "production-broker-1" --profiles local,shared-k8s
```

For defaults it should use the broker info in `~/.scion/`

This command orchestrates the full registration flow:

1. **Creates Broker Record** - Calls `POST /api/v1/brokers` using the user's existing Hub credentials
2. **Extracts Join Token** - Receives the `joinToken` from the response
3. **Completes Join** - Immediately calls `POST /api/v1/brokers/join` with the token
4. **Stores Credentials** - Saves the returned secret to local credential storage

The Runtime Broker API exposes a `JoinHub` method that performs steps 3-4:

```go
// pkg/runtimebroker/api.go
func (h *RuntimeBroker) JoinHub(ctx context.Context, brokerID, joinToken string) error
```

For remote or headless brokers where the user cannot run commands directly, the manual two-step flow (user creates broker, operator provides join token) remains available.

### 3.4 Hub Endpoints

```
POST /api/v1/brokers
Authorization: Bearer <user-token>
Request:
{
  "name": "production-broker-1",
  "capabilities": ["docker", "kubernetes"],
  "labels": { "region": "us-west-2" }
}

Response:
{
  "brokerId": "broker-uuid-123",
  "joinToken": "scion_join_AbCdEf123456...",
  "expiresAt": "2025-01-30T13:00:00Z"
}
```

```
POST /api/v1/brokers/join
Request:
{
  "brokerId": "broker-uuid-123",
  "joinToken": "scion_join_AbCdEf123456...",
  "hostname": "prod-broker-1.example.com",
  "version": "1.0.0",
  "capabilities": ["docker"]
}

Response:
{
  "secretKey": "base64-encoded-256-bit-key",
  "hubEndpoint": "https://hub.scion.example.com",
  "brokerId": "broker-uuid-123"
}
```

### 3.5 Secret Storage

**Hub Side:**
- Initial storage in filesystem at ~/.scion/hub-secrets.json
- Future implementation: Store secret hash in database: `broker_secrets(broker_id, secret_hash, created_at, rotated_at)`
- Keep plaintext secret only in memory during validation
- Support secret rotation with grace period

**Runtime Broker Side:**
- Initial: JSON file at `~/.scion/broker-credentials.json` (mode 0600)
- Production: Google Secret Manager or HashiCorp Vault
- Structure:
  ```json
  {
    "brokerId": "broker-uuid-123",
    "secretKey": "base64-encoded-key",
    "hubEndpoint": "https://hub.scion.example.com",
    "registeredAt": "2025-01-30T12:00:00Z"
  }
  ```

---

## 4. Phase 2: Ongoing Authentication (Request Signing)

Once registered, all requests between Runtime Broker and Hub are HMAC-signed.

### 4.1 Signing Process (Sender)

1. **Prepare Metadata**
   - Generate timestamp $T$ (current UTC time, RFC 3339)
   - Generate random nonce $N$ (16 bytes, base64-encoded)

2. **Build Canonical String**
   - Concatenate request elements in strict order:
   ```
   S = METHOD + "\n" +
       PATH + "\n" +
       QUERY (sorted) + "\n" +
       TIMESTAMP + "\n" +
       NONCE + "\n" +
       CONTENT_HASH
   ```
   - `CONTENT_HASH` = SHA-256 of request body (empty string hash if no body)

3. **Compute Signature**
   ```
   Signature = HMAC-SHA256(K_s, S)
   ```

4. **Attach Headers**
   ```
   X-Scion-Broker-ID: broker-uuid-123
   X-Scion-Timestamp: 2025-01-30T12:00:00Z
   X-Scion-Nonce: random-base64-nonce
   X-Scion-Signature: computed-signature-base64
   ```

### 4.2 Verification Process (Receiver)

1. **Extract Headers**
   - Parse all `X-Scion-*` headers
   - Reject if any required header is missing

2. **Clock Skew Check**
   - Parse timestamp and compare to current time
   - Reject if difference > 5 minutes (configurable)

3. **Nonce Validation** (Optional, for strict replay prevention)
   - Check nonce against recent-nonce cache
   - Reject if nonce was seen within the clock skew window
   - Store nonce with expiry

4. **Secret Retrieval**
   - Look up secret by `X-Scion-Broker-ID`
   - Reject if broker not found or deactivated

5. **Signature Verification**
   - Rebuild canonical string from received request
   - Compute expected signature
   - Use `hmac.Equal()` for constant-time comparison
   - Reject if signatures don't match

### 4.3 Go Implementation

```go
// pkg/hub/brokerauth.go

type BrokerAuthConfig struct {
    MaxClockSkew time.Duration // Default: 5 minutes
    EnableNonceCache bool      // Enable strict replay prevention
}

type BrokerAuthMiddleware struct {
    secrets BrokerSecretStore
    config  BrokerAuthConfig
    nonces  *NonceCache // Optional
}

func (m *BrokerAuthMiddleware) Middleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        brokerID := r.Header.Get("X-Scion-Broker-ID")
        timestamp := r.Header.Get("X-Scion-Timestamp")
        nonce := r.Header.Get("X-Scion-Nonce")
        signature := r.Header.Get("X-Scion-Signature")

        // Validate presence
        if brokerID == "" || timestamp == "" || signature == "" {
            writeBrokerAuthError(w, "missing required authentication headers")
            return
        }

        // Parse and validate timestamp
        ts, err := time.Parse(time.RFC3339, timestamp)
        if err != nil {
            writeBrokerAuthError(w, "invalid timestamp format")
            return
        }
        if time.Since(ts).Abs() > m.config.MaxClockSkew {
            writeBrokerAuthError(w, "timestamp outside acceptable window")
            return
        }

        // Optional: Check nonce
        if m.config.EnableNonceCache && nonce != "" {
            if m.nonces.Seen(nonce) {
                writeBrokerAuthError(w, "duplicate nonce (possible replay)")
                return
            }
            m.nonces.Add(nonce, m.config.MaxClockSkew)
        }

        // Get secret
        secret, err := m.secrets.GetSecret(r.Context(), brokerID)
        if err != nil {
            writeBrokerAuthError(w, "unknown broker")
            return
        }

        // Build canonical string and verify
        canonical := buildCanonicalString(r, timestamp, nonce)
        expected := computeHMAC(secret, canonical)

        providedSig, err := base64.StdEncoding.DecodeString(signature)
        if err != nil || !hmac.Equal(expected, providedSig) {
            writeBrokerAuthError(w, "invalid signature")
            return
        }

        // Add broker context
        ctx := context.WithValue(r.Context(), brokerContextKey{}, &BrokerIdentity{
            ID: brokerID,
        })
        next.ServeHTTP(w, r.WithContext(ctx))
    })
}

func buildCanonicalString(r *http.Request, timestamp, nonce string) []byte {
    var buf bytes.Buffer
    buf.WriteString(r.Method)
    buf.WriteByte('\n')
    buf.WriteString(r.URL.Path)
    buf.WriteByte('\n')
    buf.WriteString(canonicalQuery(r.URL.Query()))
    buf.WriteByte('\n')
    buf.WriteString(timestamp)
    buf.WriteByte('\n')
    buf.WriteString(nonce)
    buf.WriteByte('\n')
    buf.WriteString(contentHash(r))
    return buf.Bytes()
}

func computeHMAC(secret, data []byte) []byte {
    h := hmac.New(sha256.New, secret)
    h.Write(data)
    return h.Sum(nil)
}
```

---

## 5. Bidirectional Authentication

Because both the Hub and Runtime Broker possess $K_s$, either party can initiate authenticated requests.

### 5.1 Runtime Broker → Hub

The Runtime Broker communicates with the Hub for broker-level operations:
- Broker heartbeat and health status
- Broker resource availability (CPU, memory, disk)
- Agent lifecycle events (started, stopped, crashed)
- Grove registration updates

> **Note:** Agent-level status updates (thinking, executing, waiting for input) are sent directly by sciontool running *inside* the agent container using agent JWT authentication, not by the Runtime Broker. See [sciontool-auth.md](sciontool-auth.md) for agent authentication.

### 5.2 Hub → Runtime Broker

The Hub can push commands to registered brokers:
- Agent provisioning requests
- Agent termination commands
- Configuration updates
- Secret rotation notifications

**Runtime Broker Endpoints:**
```
POST /api/v1/agents/provision   # Start a new agent
DELETE /api/v1/agents/{id}      # Stop an agent
POST /api/v1/config/reload      # Reload configuration
POST /api/v1/secrets/rotate     # Accept new secret
```

### 5.3 Broker Endpoint Security

Runtime Brokers must:
1. Bind to localhost or private network only (not public internet)
2. Validate Hub signatures using the same HMAC mechanism
3. Verify the requesting Hub matches the registered `hubEndpoint`

---

## 6. Secret Rotation

Secrets should be rotated periodically or on security events.

### 6.1 Rotation Flow

```
┌─────────────┐                    ┌──────────────┐
│     Hub     │                    │ Runtime Broker │
└─────────────┘                    └──────────────┘
       │                                  │
       │ POST /api/v1/secrets/rotate      │
       │ (signed with current secret)     │
       │ { newSecret: "base64..." }       │
       │─────────────────────────────────►│
       │                                  │
       │                                  │ [Store new secret]
       │                                  │ [Keep old for grace period]
       │                                  │
       │           200 OK                 │
       │◄─────────────────────────────────│
       │                                  │
       │ [Mark old secret deprecated]     │
       │                                  │
       │ ... grace period (e.g., 1 hour) ...
       │                                  │
       │ [Remove old secret]              │
       │                                  │
```

### 6.2 Dual-Secret Validation

During the grace period, the Hub accepts signatures from either secret:

```go
func (m *BrokerAuthMiddleware) verifyWithRotation(brokerID string, canonical, signature []byte) bool {
    secrets, _ := m.secrets.GetSecrets(brokerID) // Returns current + deprecated
    for _, secret := range secrets {
        expected := computeHMAC(secret.Key, canonical)
        if hmac.Equal(expected, signature) {
            return true
        }
    }
    return false
}
```

---

## 7. Configuration

### 7.1 Hub Configuration

```yaml
server:
  brokerAuth:
    enabled: true
    maxClockSkew: 5m
    enableNonceCache: true
    nonceCacheTTL: 10m
    secretRotation:
      gracePeriod: 1h
      autoRotateInterval: 30d  # 0 to disable
    joinToken:
      expiry: 1h
      length: 32  # bytes
```

### 7.2 Runtime Broker Configuration

```yaml
broker:
  hub:
    endpoint: "https://hub.scion.example.com"
  credentials:
    file: "~/.scion/broker-credentials.json"
    # OR for production:
    # secretManager: "projects/my-project/secrets/scion-broker-secret"
  api:
    listenAddr: "127.0.0.1:9815"  # For Hub callbacks
```

---

## 8. Security Considerations

### 8.1 Transport Security

- All communication MUST use HTTPS
- Minimum TLS 1.2, prefer TLS 1.3
- Certificate validation required (no `InsecureSkipVerify`)

### 8.2 Secret Entropy

- Secrets MUST be 256 bits (32 bytes) minimum
- Generated using `crypto/rand`
- Base64-encoded for storage/transmission

### 8.3 Clock Synchronization

- Both Hub and Runtime Brokers should use NTP
- 5-minute skew tolerance accommodates minor drift
- Larger skew may indicate MITM or misconfiguration

### 8.4 Nonce Cache Considerations

- Nonce cache prevents replay within clock skew window
- Memory cost: ~50 bytes per nonce × requests per window
- Optional for lower-security deployments

### 8.5 Audit Logging

Log all broker authentication events:
```go
type BrokerAuthEvent struct {
    EventType  string    `json:"eventType"`  // register, join, auth_success, auth_failure
    BrokerID   string    `json:"brokerId"`
    IPAddress  string    `json:"ipAddress"`
    Success    bool      `json:"success"`
    FailReason string    `json:"failReason,omitempty"`
    Timestamp  time.Time `json:"timestamp"`
}
```

---

## 9. Open Questions

None pending.

---

## 10. Resolved Questions

### 10.1 Registration Authorization

**Question:** What permission level should be required to register a new Runtime Broker?

- **Option A:** Admin-only (restrictive)
- **Option B:** New "broker-manager" role
- **Option C:** Any authenticated user (permissive, for dev/testing)

**Decision:** Admin-only for initial implementation. Future RBAC system may introduce a dedicated "broker-manager" role for delegated broker administration.

### 10.2 Broker Identity Verification

**Question:** Should we verify broker identity beyond the join token during registration?

Considerations:
- Should brokers provide a CSR for mutual TLS?
- Should brokers prove control of a hostname/IP?
- Is the join token sufficient for trust establishment?

**Decision:** Admin-issued join token is sufficient for trust establishment. The join token is short-lived and requires admin authorization to create, establishing a chain of trust. mTLS may be considered as a future enhancement for high-security deployments.

### 10.3 Nonce Storage Backend

**Question:** What backend should store nonces for replay prevention?

- **Option A:** In-memory (simple, lost on restart)
- **Option B:** Filesystem (`~/.scion/`) for persistence
- **Option C:** Redis/Memcached (distributed, for HA)

**Decision:** In-memory storage is sufficient for nonce tracking. Nonces only need to be tracked within the clock skew window (5 minutes), so persistence across restarts is not required. If a restart occurs, the timestamp validation alone provides adequate replay protection for the brief window where old nonces might be reused.

### 10.4 Hub-to-Broker Communication Model

**Question:** Should the Hub push commands to brokers, or should brokers poll?

| Approach | Pros | Cons |
|----------|------|------|
| Push (HTTP to broker) | Low latency, immediate commands | Requires broker to expose endpoint |
| Poll (broker queries Hub) | No inbound firewall rules needed | Latency, polling overhead |
| WebSocket (persistent) | Real-time, single connection | Connection management complexity |

**Decision:** WebSocket-based persistent connection, initiated by the broker. This overcomes firewall restrictions (brokers behind NAT can still receive commands) while providing real-time bidirectional communication. See the WebSocket design documentation for connection management details.

### 10.5 WebSocket Message Authentication

**Question:** Once a WebSocket connection is established with HMAC authentication, should individual messages over that connection require per-message authentication?

| Approach | Security | Performance | Complexity |
|----------|----------|-------------|------------|
| **Per-message HMAC** | Highest - each command independently verified | Overhead per message | Higher - must sign/verify each message |
| **Session-based trust** | Connection authenticated once, messages trusted | Minimal overhead | Lower - no per-message crypto |
| **Hybrid** | Critical commands signed, routine messages trusted | Moderate | Medium - classify message criticality |

Arguments for session-based trust:
- WebSocket runs over TLS, providing transport-level integrity
- Initial connection is HMAC-authenticated, establishing broker identity
- Connection hijacking would require TLS compromise
- Similar to how SSH trusts commands after key exchange

Arguments for per-message signing:
- Defense in depth against potential TLS vulnerabilities
- Audit trail with cryptographic proof per command
- Protects against compromised Hub process memory

**Decision:** Session-based trust for Hub→Broker commands over the established WebSocket connection. The WebSocket is strictly for Hub-initiated commands to brokers (agent provisioning, termination, config updates). Broker→Hub requests that require authorization (status updates, API calls) must use standard HMAC-authenticated HTTP requests and should **not** flow over this WebSocket channel. This maintains proper authorization semantics while avoiding per-message overhead for trusted command delivery.

### 10.6 Secret Rotation Trigger

**Question:** What should trigger automatic secret rotation?

- Time-based (every N days)?
- Event-based (security incident, personnel change)?
- Manual only?

**Decision:** Manual rotation with optional time-based auto-rotation. Administrators can trigger rotation on-demand (security incidents, personnel changes), with optional configuration for automatic rotation at defined intervals (e.g., 30 days).

### 10.7 Multi-Hub Support

**Question:** Can a Runtime Broker be registered with multiple Hubs?

- If yes, how are secrets managed per-Hub?
- What prevents Hub A from impersonating Hub B?

**Decision:** Single Hub per broker for initial implementation, with multi-Hub support planned for the future.

Future multi-Hub design considerations:
- Broker ID remains consistent across Hubs (same broker identity)
- Each Hub relationship has a unique shared secret
- Credential storage must be keyed by Hub endpoint: `~/.scion/broker-credentials/{hub-id}.json`
- Hub impersonation prevented by unique secrets and endpoint verification

Storage systems should be designed to accommodate per-Hub credentials from the start.

### 10.8 Broker Deactivation and Cleanup

**Question:** What happens when a broker is deactivated?

- Should running agents be terminated immediately?
- How long should the secret be retained for audit purposes?
- Should there be a "quarantine" state before full deletion?

**Decision:** On broker deactivation:
1. Running agents are terminated immediately (Hub sends termination commands if broker is reachable)
2. Broker secret is marked inactive (authentication attempts rejected)
3. Secret hash retained for 30 days for audit trail purposes
4. Full deletion after retention period

### 10.9 Integration with Agent Auth

**Question:** How does broker authentication relate to agent token issuance?

When the Hub provisions an agent on a broker:
1. Hub authenticates to broker via HMAC (or over authenticated WebSocket)
2. Broker starts agent container with... what token?

Options:
- Hub sends pre-signed agent JWT in provision request
- Broker requests agent JWT from Hub after container starts
- Agent bootstraps its own token via Hub API

**Decision:** Hub includes pre-signed agent JWT in the provision request payload.

Rationale:
- Runtime Brokers operate at a higher trust level than individual agents
- The trust model assumes brokers will not abuse access to agent credentials
- Pre-signed tokens eliminate a round-trip and simplify agent startup
- Agent tokens are scoped to specific agent IDs, limiting blast radius if compromised
- Broker compromise is a more serious security event that would be addressed separately

---

## 11. Implementation Checklist

### Phase 1: Core Infrastructure ✓
- [x] Define `BrokerSecretStore` interface (`pkg/store/store.go`)
- [x] ~~Implement in-memory secret store (dev/testing)~~ (skipped - SQLite with `:memory:` sufficient for testing)
- [x] Implement SQLite secret store (`pkg/store/sqlite/brokersecret.go`)
- [x] Create broker registration endpoints (`POST /brokers`, `POST /brokers/join`) (`pkg/hub/handlers_brokers.go`)
- [x] Implement `BrokerAuthMiddleware` for Hub (`pkg/hub/brokerauth.go`)

**Implementation Notes (Phase 1):**
- Timestamp format uses Unix epoch (seconds) rather than RFC 3339 for simpler parsing
- `BrokerAuthService` provides both middleware and `SignRequest()` helper for clients
- Join tokens use `scion_join_` prefix with base64-encoded random bytes
- Nonce cache is optional and disabled by default (`EnableNonceCache: false`)
- Secret keys are 256-bit (32 bytes) generated via `crypto/rand`
- Database migration V8 adds `broker_secrets` and `broker_join_tokens` tables with FK cascade delete

### Phase 2: Runtime Broker Integration ✓
- [x] Add HMAC signing to `hubclient` package (`pkg/apiclient/hmac.go`, `hubclient.WithHMACAuth()`)
- [x] Implement local credential storage (`pkg/brokercredentials/store.go`)
- [x] Add broker-side signature verification (`pkg/runtimebroker/brokerauth.go`)
- [x] Implement heartbeat/status reporting (`pkg/runtimebroker/heartbeat.go`)

**Implementation Notes (Phase 2):**
- `HMACAuth` implements `apiclient.Authenticator` for signing outgoing requests
- `BuildCanonicalString` and `ComputeHMAC` are exported for use by both client and server
- `BrokerCredentials` stored in `~/.scion/broker-credentials.json` with 0600 permissions
- `HeartbeatService` runs background goroutine sending heartbeats at configurable interval (default 30s)
- `BrokerAuthMiddleware` verifies incoming Hub requests using shared secret
- Server integration loads credentials on startup and configures HMAC auth automatically

### Phase 3: Bidirectional Communication ✓
- [x] Add Hub→Broker HTTP client with HMAC signing (`pkg/hub/brokerclient.go`)
- [x] Update RuntimeBrokerClient interface to include brokerID (`pkg/hub/server.go`)
- [x] Update HTTPAgentDispatcher to pass brokerID for auth (`pkg/hub/httpdispatcher.go`)
- [x] Add strict mode config for Runtime Broker (`pkg/runtimebroker/server.go`)

**Implementation Notes (Phase 3):**
- `AuthenticatedBrokerClient` wraps HTTP client with HMAC signing using `apiclient.HMACAuth`
- `RuntimeBrokerClient` interface methods now take `brokerID` as first parameter after `ctx`
- `HTTPRuntimeBrokerClient` ignores brokerID (for backward compatibility), `AuthenticatedBrokerClient` uses it for secret lookup
- `CreateAuthenticatedDispatcher()` helper on Hub Server creates dispatcher with authenticated client
- `BrokerAuthStrictMode` config on Runtime Broker: when true, requires all requests to be authenticated; when false (default), allows unauthenticated requests during transition
- Graceful degradation: if secret lookup fails, request proceeds without signature (logged as warning)

### Phase 4: Production Hardening ✓
- [x] Enable nonce cache by default (`pkg/hub/brokerauth.go` - `EnableNonceCache: true`)
- [x] Implement secret rotation flow (`POST /api/v1/brokers/{id}/rotate-secret`)
- [x] Add dual-secret validation support (`GetActiveSecrets`, `ValidateBrokerSignatureWithRotation`)
- [ ] Add Google Secret Manager integration (deferred - local file storage sufficient for now)
- [x] Add comprehensive audit logging (`pkg/hub/audit.go`)
- [x] Add metrics (`pkg/hub/metrics.go`, `/metrics` endpoint)

**Implementation Notes (Phase 4):**
- Nonce cache now enabled by default in `DefaultBrokerAuthConfig()` for replay attack prevention
- `RotateBrokerSecret()` generates new secret and updates existing record; full dual-secret with schema migration deferred
- `GetActiveSecrets()` returns both active and deprecated secrets for grace period validation
- Rotation endpoint at `POST /api/v1/brokers/{id}/rotate-secret` allows admin or broker self-rotation
- `LogAuditLogger` logs to standard logger; implements `AuditLogger` interface for custom backends
- `AuditableBrokerAuthMiddleware` wraps auth with audit logging for success/failure events
- Helper functions: `LogRegistrationEvent`, `LogJoinEvent`, `LogRotateEvent` for handler integration
- `BrokerAuthMetrics` tracks counters (auth attempts, registrations, joins, rotations, dispatches) and latency percentiles
- `/metrics` endpoint returns JSON snapshot of all metrics
- Note: Current schema uses `broker_id` as primary key (one secret per broker); true dual-secret rotation requires schema migration to support multiple secrets per broker

---

## 12. Related Documents

- [Server Auth Design](server-auth-design.md) - Hub authentication for users and agents
- [Auth Overview](auth-overview.md) - Identity model and token types
- [Agent Authentication](sciontool-auth.md) - Agent-to-Hub JWT
- [Hosted Architecture](../hosted-architecture.md) - System context
- [RuntimeBroker Websockets](../runtimebroker-websocket.md) - RuntimeBroker websocket architecture details
