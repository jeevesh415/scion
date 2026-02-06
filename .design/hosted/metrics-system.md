# Hosted Scion Metrics System Design

## Status
**In Progress** - Milestones 1-2 complete. Milestones 3-4 pending.

## 1. Overview

This document defines the metrics and observability architecture for the Hosted Scion platform. The design synthesizes research on LLM agent telemetry patterns (Codex, Gemini CLI, OpenCode) with the Hosted Scion architecture to create a unified observability strategy.

### Design Principles

1. **Sciontool as Primary Collector**: The `sciontool` binary running inside each agent container serves as the single point of telemetry collection, normalization, and forwarding.

2. **Cloud-Native Observability Backend**: Raw telemetry data (logs, traces, metrics) is forwarded to a dedicated cloud-based observability platform (e.g., Google Cloud Observability, Datadog, Honeycomb). The Hub does not become a general-purpose metrics or logging backend.

3. **Hub for High-Level Aggregates Only**: The Hub receives lightweight, pre-aggregated session and agent metrics for dashboard display, not raw telemetry streams. It can also fetch query-based aggregate data or recent logs from the cloud observability backend for presentation layer use.

4. **Configurable Filtering**: Sciontool provides event filtering to control volume, respect privacy settings, and honor debug mode configurations.

5. **Progressive Enhancement**: Initial implementation focuses on core metrics flow; advanced analytics via the web UI will come in a future phase.

---

## 2. Architecture

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           Agent Container                                   │
│                                                                             │
│  ┌─────────────────────┐                                                   │
│  │  Agent Process      │                                                   │
│  │  (Claude/Gemini)    │                                                   │
│  │                     │                                                   │
│  │  Emits:             │                                                   │
│  │  - OTLP (native)    │──────────┐                                        │
│  │  - JSON logs        │          │                                        │
│  │  - Hook events      │          │                                        │
│  └─────────────────────┘          │                                        │
│           │                       │                                        │
│           │ Hook calls            │ OTLP                                   │
│           ▼                       ▼                                        │
│  ┌─────────────────────────────────────────────────────────────┐           │
│  │                     Sciontool                                │           │
│  │                                                              │           │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │           │
│  │  │ Event        │  │ OTLP         │  │ Aggregation  │       │           │
│  │  │ Normalizer   │  │ Receiver     │  │ Engine       │       │           │
│  │  │              │  │ :4317        │  │              │       │           │
│  │  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │           │
│  │         │                 │                 │                │           │
│  │         └─────────────────┼─────────────────┘                │           │
│  │                           │                                  │           │
│  │                    ┌──────┴──────┐                          │           │
│  │                    │   Filter    │                          │           │
│  │                    │   Engine    │                          │           │
│  │                    └──────┬──────┘                          │           │
│  │                           │                                  │           │
│  │         ┌─────────────────┼─────────────────┐               │           │
│  │         │                 │                 │               │           │
│  │         ▼                 ▼                 ▼               │           │
│  │  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │           │
│  │  │ Cloud        │  │ Hub          │  │ Local        │       │           │
│  │  │ Forwarder    │  │ Reporter     │  │ Debug        │       │           │
│  │  │              │  │              │  │ Output       │       │           │
│  │  └──────┬───────┘  └──────┬───────┘  └──────────────┘       │           │
│  │         │                 │                                  │           │
│  └─────────┼─────────────────┼──────────────────────────────────┘           │
│            │                 │                                              │
└────────────┼─────────────────┼──────────────────────────────────────────────┘
             │                 │
             │                 │
             ▼                 ▼
    ┌─────────────────┐  ┌─────────────────┐
    │ Cloud           │  │ Scion Hub       │
    │ Observability   │  │                 │
    │ Backend         │  │ Stores:         │
    │                 │  │ - Session       │
    │ - Full traces   │  │   summaries     │
    │ - All logs      │  │ - Agent metrics │
    │ - Raw metrics   │  │ - Activity      │
    │                 │  │                 │
    └─────────────────┘  └─────────────────┘
             │
             │ Query API
             ▼
    ┌─────────────────┐
    │ Web UI          │
    │ (Future)        │
    │                 │
    │ - Deep analytics│
    │ - Trace viewer  │
    │ - Log search    │
    └─────────────────┘
```

---

## 3. Sciontool as Primary Collector

### 3.1 Data Ingestion

Sciontool receives telemetry from agent processes through multiple channels:

| Channel | Source | Format | Example Events |
|---------|--------|--------|----------------|
| **OTLP Receiver** | Agents with native OTel (Codex, OpenCode) | OTLP gRPC/HTTP | Spans, metrics, logs |
| **Hook Events** | Harness hook calls | JSON via CLI args | `tool-start`, `tool-end`, `prompt-submit` |
| **Session Files** | Gemini CLI session JSON | File watch/poll | Token counts, tool calls |
| **Stdout/Stderr** | Agent process output | Line-based text | Structured log lines |

### 3.2 Event Normalization

All ingested data is normalized to a common schema before processing. This enables harness-agnostic analytics.

#### Normalized Event Schema

```json
{
  "timestamp": "2026-02-02T10:30:00Z",
  "event_type": "agent.tool.call",
  "session_id": "uuid",
  "agent_id": "agent-abc123",
  "grove_id": "grove-xyz",

  "attributes": {
    "tool_name": "shell_execute",
    "duration_ms": 1250,
    "success": true,
    "model": "gemini-2.0-pro"
  },

  "metrics": {
    "tokens_input": 1500,
    "tokens_output": 450,
    "tokens_cached": 800
  }
}
```

#### Event Type Catalog

Based on the normalized metrics research, sciontool recognizes these event types:

| Event Type | Category | Description |
|------------|----------|-------------|
| `agent.session.start` | Lifecycle | Agent session initiated |
| `agent.session.end` | Lifecycle | Agent session completed |
| `agent.user.prompt` | Interaction | User input received |
| `agent.response.complete` | Interaction | Agent response finished |
| `agent.tool.call` | Tool Use | Tool execution started |
| `agent.tool.result` | Tool Use | Tool execution completed |
| `agent.approval.request` | Interaction | Permission requested from user |
| `gen_ai.api.request` | LLM | API call to LLM provider |
| `gen_ai.api.response` | LLM | Response received from LLM |
| `gen_ai.api.error` | LLM | API error occurred |

### 3.3 Dialect Parsing

Each harness emits events in its native format. Sciontool's dialect parsers translate these to the normalized schema.

```
┌──────────────────────────────────────────────────────────┐
│                    Dialect Parsers                       │
├──────────────────────────────────────────────────────────┤
│                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐      │
│  │ Claude      │  │ Gemini      │  │ OpenCode    │      │
│  │ Dialect     │  │ Dialect     │  │ Dialect     │      │
│  │             │  │             │  │             │      │
│  │ Parses:     │  │ Parses:     │  │ Parses:     │      │
│  │ - CC hooks  │  │ - Settings  │  │   JSON      │      │
│  │   events    │  │   JSON      │  │ - OTEL      │      │
│  │             │  │ - OTEL      │  │   events    │      │
│  │             │  │ - Session   │  │             │      │
│  │             │  │   Files     │  │             │      │
│  └─────────────┘  └─────────────┘  └─────────────┘      │
│         │                │                │              │
│         └────────────────┼────────────────┘              │
│                          ▼                               │
│              ┌─────────────────────┐                     │
│              │ Normalized Event    │                     │
│              │ Stream              │                     │
│              └─────────────────────┘                     │
└──────────────────────────────────────────────────────────┘
```

---

## 4. Data Destinations

### 4.1 Cloud Observability Backend (Primary)

The majority of telemetry data is forwarded to a cloud-based observability platform. This enables:

- Full-fidelity trace analysis
- Log search and aggregation
- Long-term metric storage
- Advanced querying and dashboards

**Supported Backends (Initial):**

| Backend | Protocol | Use Case |
|---------|----------|----------|
| Google Cloud Observability | OTLP | GCP-native deployments |
| Generic OTLP Collector | OTLP gRPC/HTTP | Self-hosted, multi-cloud |

#### Forward Configuration

```yaml
# sciontool config (injected via env or config file)
telemetry:
  cloud:
    enabled: true
    endpoint: "otel-collector.example.com:4317"
    protocol: "grpc"  # grpc, http
    headers:
      Authorization: "Bearer ${OTEL_API_KEY}"

    # Batch settings for efficiency
    batch:
      maxSize: 512
      timeout: "5s"

    # TLS configuration
    tls:
      enabled: true
      insecureSkipVerify: false
```

#### Data Forwarded to Cloud

| Data Type | Volume | Retention (typical) |
|-----------|--------|---------------------|
| Traces | All spans | 14-30 days |
| Logs | All agent logs | 30-90 days |
| Metrics | All counters/histograms | 13 months |

### 4.2 Hub Reporting (Aggregated)

The Hub receives only lightweight, pre-aggregated data for display in the web dashboard. This keeps the Hub focused on its core responsibility: state management.

**Data Sent to Hub:**

| Metric | Aggregation | Frequency |
|--------|-------------|-----------|
| Session summary | Per-session | On session end |
| Token usage | Per-session totals | On session end |
| Tool call counts | Per-session by tool | On session end |
| Agent status | Current state | On change |
| Error counts | Rolling 1-hour window | Every 5 minutes |

#### Hub Reporting Protocol

Sciontool reports to the Hub via the existing daemon heartbeat channel, extending the payload:

```json
{
  "type": "agent_metrics",
  "agent_id": "agent-abc123",
  "timestamp": "2026-02-02T10:35:00Z",

  "session": {
    "id": "session-uuid",
    "started_at": "2026-02-02T10:00:00Z",
    "ended_at": "2026-02-02T10:35:00Z",
    "status": "completed",
    "turn_count": 15,
    "model": "gemini-2.0-pro"
  },

  "tokens": {
    "input": 45000,
    "output": 12000,
    "cached": 30000,
    "reasoning": 5000
  },

  "tools": {
    "shell_execute": { "calls": 8, "success": 7, "error": 1 },
    "read_file": { "calls": 25, "success": 25, "error": 0 },
    "write_file": { "calls": 4, "success": 4, "error": 0 }
  },

  "languages": ["TypeScript", "Go", "Markdown"]
}
```

#### Hub Storage

The Hub stores these summaries in a dedicated table (not raw events):

```sql
CREATE TABLE agent_session_metrics (
    id              TEXT PRIMARY KEY,
    agent_id        TEXT NOT NULL,
    grove_id        TEXT NOT NULL,
    session_id      TEXT NOT NULL,

    started_at      TIMESTAMP NOT NULL,
    ended_at        TIMESTAMP,
    status          TEXT,

    turn_count      INTEGER,
    model           TEXT,

    tokens_input    INTEGER,
    tokens_output   INTEGER,
    tokens_cached   INTEGER,
    tokens_reasoning INTEGER,

    tool_calls      JSONB,  -- {"tool_name": {"calls": N, "success": N, "error": N}}
    languages       TEXT[], -- ["TypeScript", "Go"]

    -- cost_estimate   DECIMAL(10, 6), -- Postponed to future phase

    created_at      TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    FOREIGN KEY (agent_id) REFERENCES agents(id),
    FOREIGN KEY (grove_id) REFERENCES groves(id)
);

CREATE INDEX idx_session_metrics_agent ON agent_session_metrics(agent_id);
CREATE INDEX idx_session_metrics_grove ON agent_session_metrics(grove_id);
CREATE INDEX idx_session_metrics_time ON agent_session_metrics(started_at);
```

### 4.3 Local Debug Output

In debug mode or when cloud forwarding is disabled, sciontool can output telemetry locally for troubleshooting.

| Output | Trigger | Format |
|--------|---------|--------|
| Console (stderr) | `SCION_LOG_LEVEL=debug` | Structured text |
| File | `telemetry.local.file` configured | JSONL |
| Debug endpoint | `telemetry.local.endpoint` | OTLP to localhost |

---

## 5. Filtering and Sampling

Sciontool provides configurable filtering to manage telemetry volume and respect privacy requirements.

### 5.1 Filter Configuration

```yaml
telemetry:
  filter:
    # Global enable/disable
    enabled: true

    # Respect debug mode (SCION_LOG_LEVEL)
    respectDebugMode: true

    # Event type filtering
    events:
      # Include list (if set, only these are forwarded)
      include: []

      # Exclude list (these are never forwarded)
      exclude:
        - "agent.user.prompt"  # Privacy: don't forward user prompts by default

    # Attribute filtering
    attributes:
      # Fields to redact (replaced with "[REDACTED]")
      redact:
        - "prompt"
        - "user.email"
        - "tool_output"  # May contain sensitive file contents

      # Fields to hash (replaced with SHA256 hash)
      hash:
        - "session_id"  # For correlation without exposing raw IDs

    # Sampling (for high-volume events)
    sampling:
      # Default sample rate (1.0 = 100%)
      default: 1.0

      # Per-event-type rates
      rates:
        "gen_ai.api.request": 0.1  # Sample 10% of API requests
        "agent.tool.result": 0.5   # Sample 50% of tool results
```

### 5.2 Debug Mode Behavior

When debug mode is enabled (`SCION_LOG_LEVEL=debug`):

1. All filtering is bypassed for local output
2. Sampling rates are ignored for local output
3. Cloud forwarding still respects privacy filters (redaction)
4. Additional diagnostic events are emitted

### 5.3 Privacy Defaults

Out of the box, sciontool applies these privacy-preserving defaults:

| Data | Default Behavior | Rationale |
|------|------------------|-----------|
| User prompts | Redacted | May contain sensitive instructions |
| Tool output | Redacted | May contain file contents, credentials |
| User email | Redacted | PII |
| Session ID | Hashed | Allow correlation without exposure |
| Agent ID | Passed through | Required for routing |
| Token counts | Passed through | Non-sensitive, needed for cost tracking |

Users can opt-in to full prompt/output logging via configuration:

```yaml
telemetry:
  filter:
    attributes:
      # Override defaults to allow prompt logging
      redact: []  # Empty = no redaction
```

---

## 6. Hub Metrics API

The Hub exposes an API for retrieving aggregated metrics for display in the web UI.

### 6.1 Endpoints

#### Get Agent Metrics Summary

```
GET /api/v1/agents/{agentId}/metrics/summary
```

**Response:**
```json
{
  "agent_id": "agent-abc123",
  "period": "24h",

  "sessions": {
    "total": 15,
    "completed": 14,
    "errored": 1
  },

  "tokens": {
    "input": 450000,
    "output": 120000,
    "cached": 300000
  },

  "top_tools": [
    { "name": "read_file", "calls": 250, "success_rate": 1.0 },
    { "name": "shell_execute", "calls": 80, "success_rate": 0.95 },
    { "name": "write_file", "calls": 40, "success_rate": 1.0 }
  ],

  "languages": ["TypeScript", "Go", "Python"]
}
```

#### Get Grove Metrics Summary

```
GET /api/v1/groves/{groveId}/metrics/summary
```

Returns aggregated metrics across all agents in the grove.

#### Get Metrics Time Series

```
GET /api/v1/groves/{groveId}/metrics/timeseries?metric=tokens.input&period=7d&interval=1h
```

Returns time-bucketed metric values for charting.

### 6.2 What the Hub Does NOT Provide

The Hub explicitly does **not** provide:

- Raw log search/retrieval
- Trace viewing
- Full-fidelity metric queries
- Log aggregation pipelines

These capabilities are delegated to the cloud observability backend.

---

## 7. Future: Web UI Observability Features

In a future phase, the web UI will provide deeper observability by fetching data from the cloud backend.

### 7.1 Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        Web UI                               │
│                                                             │
│  ┌───────────────────┐  ┌───────────────────────────────┐  │
│  │ Dashboard         │  │ Deep Analytics (Future)       │  │
│  │                   │  │                               │  │
│  │ Data from: Hub    │  │ Data from: Cloud Backend      │  │
│  │                   │  │                               │  │
│  │ - Session counts  │  │ - Trace viewer                │  │
│  │ - Token totals    │  │ - Log search                  │  │
│  │ - Cost estimates  │  │ - Custom queries              │  │
│  │ - Agent status    │  │ - Anomaly detection           │  │
│  └───────────────────┘  └───────────────────────────────┘  │
│           │                          │                      │
└───────────┼──────────────────────────┼──────────────────────┘
            │                          │
            ▼                          ▼
     ┌─────────────┐          ┌─────────────────────┐
     │  Scion Hub  │          │ Cloud Observability │
     │  API        │          │ Query API           │
     └─────────────┘          └─────────────────────┘
```

### 7.2 Planned Features

| Feature | Data Source | Priority |
|---------|-------------|----------|
| Session list with metrics | Hub | P1 |
| Token usage charts | Hub | P1 |
| Cost tracking dashboard | Hub | P1 |
| Trace waterfall view | Cloud Backend | P2 |
| Log search | Cloud Backend | P2 |
| Tool execution timeline | Cloud Backend | P2 |
| Error analysis | Cloud Backend | P3 |
| Custom metric queries | Cloud Backend | P3 |

### 7.3 Cloud Backend Integration

The web UI will authenticate to the cloud backend using one of:

1. **Proxy through Hub**: Hub makes cloud queries on behalf of UI (simpler auth)
2. **Direct with short-lived tokens**: Hub issues tokens for UI to query cloud directly

The specific approach will be determined based on the chosen cloud backend.

---

## 8. Implementation Phases

### Phase 1: Core Telemetry Pipeline

**Goal:** Establish basic telemetry flow from agents to cloud backend.

| Task | Component | Notes |
|------|-----------|-------|
| OTLP receiver in sciontool | `pkg/sciontool/telemetry` | Receive from OTel-native agents |
| Cloud forwarder | `pkg/sciontool/telemetry` | OTLP export to cloud backend |
| Basic filtering | `pkg/sciontool/telemetry` | Event include/exclude |
| Configuration loading | `cmd/sciontool` | Environment + config file |

### Phase 2: Harness Integration

**Goal:** Capture telemetry from all harness types.

| Task | Component | Notes |
|------|-----------|-------|
| Hook event normalization | `pkg/sciontool/hooks` | Convert hook calls to events |
| Gemini session file parsing | `pkg/sciontool/hooks/dialects` | Read session-*.json |
| Claude dialect parser | `pkg/sciontool/hooks/dialects` | Parse CC hook payloads |

### Phase 3: Hub Aggregation

**Goal:** Report session summaries to Hub.

| Task | Component | Notes |
|------|-----------|-------|
| In-memory aggregation engine | `pkg/sciontool/telemetry` | Per-session accumulators |
| Hub reporter | `pkg/sciontool/hub` | Extend heartbeat protocol |
| Hub metrics storage | `pkg/hub/store` | agent_session_metrics table |
| Hub metrics API | `pkg/hub/api` | Summary endpoints |

### Phase 4: Web UI Integration

**Goal:** Display metrics in web dashboard.

| Task | Component | Notes |
|------|-----------|-------|
| Session metrics component | `web/src/client` | Display session stats |
| Token usage charts | `web/src/client` | Visualization |
| Cost tracking | `web/src/client` | Aggregate cost display |

### Phase 5: Advanced Analytics (Future)

**Goal:** Deep observability via cloud backend.

| Task | Component | Notes |
|------|-----------|-------|
| Cloud backend query proxy | `pkg/hub/api` or Web | TBD |
| Trace viewer | `web/src/client` | Embedded trace UI |
| Log search | `web/src/client` | Query interface |

## 9. System Component Logging

While `sciontool` handles telemetry for agents, the Hub and Runtime Broker servers require a robust internal logging strategy for operational observability.

### 9.1 Structured Logging with slog

All backend components (Hub, Runtime Broker) must use the Go standard library's `log/slog` package for structured logging.

- **Standardization**: Consistent key names across all components (e.g., `msg`, `level`, `time`, `component`, `trace_id`).
- **Performance**: High-performance structured logging with minimal allocation overhead.
- **Interoperability**: Standard interface allowing for easy handler swaps.

### 9.2 Log Levels and Verbosity

Logs are emitted at several levels:
- `DEBUG`: Detailed information for troubleshooting. Only emitted when explicitly enabled.
- `INFO`: Normal operational events (startup, shutdown, significant state changes).
- `WARN`: Unexpected events that don't stop the service (e.g., transient network errors).
- `ERROR`: Critical failures requiring attention.

Debug logging can be enabled globally or per-component via the `SCION_LOG_LEVEL=debug` environment variable.

### 9.3 OTel Log Bridge Architecture

In an OpenTelemetry-native environment, we employ a "Log Bridge" approach instead of custom log exporters. We use the official OTel bridge to connect the standard `log/slog` API to the OpenTelemetry Logs SDK.

- **Concept**: `slog` acts as the "frontend" API that developers interact with, while the OTel SDK acts as the "backend" that handles batching, resource attribution, and exporting to the OTLP forwarder.
- **Implementation**: We utilize the `go.opentelemetry.io/contrib/bridges/otelslog` package.

#### Implementation Pattern

1.  **Configure OTel LoggerProvider**: Initialize the OTel SDK with an OTLP exporter (pointing to the Collector/Backend).
2.  **Create Bridge Handler**: Wrap the LoggerProvider in an `otelslog.Handler`.
3.  **Set Default Logger**: Replace the global default logger or inject the bridge logger into the application context.

```go
import (
    "context"
    "log/slog"
    "go.opentelemetry.io/contrib/bridges/otelslog"
    "go.opentelemetry.io/otel/log/global"
)

func main() {
    // 1. Setup your existing OTel LoggerProvider (which points to your forwarder)
    lp := setupOTelLoggerProvider()

    // 2. Create the slog handler using the bridge
    // The "scion-hub" string defines the Instrumentation Scope
    otlpHandler := otelslog.NewHandler("scion-hub", otelslog.WithLoggerProvider(lp))

    // 3. Set as default
    logger := slog.New(otlpHandler)
    slog.SetDefault(logger)

    // 4. Usage (Always use context-aware methods for trace correlation!)
    slog.InfoContext(ctx, "processed request", "bytes", 1024)
}
```

### 9.4 Contextual Metadata

To facilitate debugging across distributed components, the following fields should be included in log records where applicable:
- `grove_id`: The ID of the grove being processed.
- `agent_id`: The ID of the agent involved.
- `request_id`: A unique ID for the incoming API request.
- `user_id`: The ID of the authenticated user.

---

## 10. Configuration Reference

### 10.1 Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `SCION_OTEL_ENDPOINT` | Cloud OTLP endpoint | (required if cloud enabled) |
| `SCION_OTEL_PROTOCOL` | OTLP protocol (grpc, http) | `grpc` |
| `SCION_OTEL_HEADERS` | Additional headers (JSON) | `{}` |
| `SCION_OTEL_INSECURE` | Skip TLS verification | `false` |
| `SCION_TELEMETRY_ENABLED` | Enable telemetry collection | `true` |
| `SCION_TELEMETRY_CLOUD_ENABLED` | Forward to cloud backend | `true` |
| `SCION_TELEMETRY_HUB_ENABLED` | Report to Hub | `true` (if hosted mode) |
| `SCION_TELEMETRY_DEBUG` | Local debug output | `false` |
| `SCION_LOG_LEVEL` | Logging verbosity | `info` |

### 10.2 Full Configuration File

```yaml
telemetry:
  enabled: true

  # Cloud forwarding
  cloud:
    enabled: true
    endpoint: "${SCION_OTEL_ENDPOINT}"
    protocol: "grpc"
    headers:
      Authorization: "Bearer ${OTEL_API_KEY}"
    tls:
      enabled: true
      insecureSkipVerify: false
    batch:
      maxSize: 512
      timeout: "5s"

  # Hub reporting
  hub:
    enabled: true  # Auto-enabled in hosted mode
    reportInterval: "30s"

  # Local debug output
  local:
    enabled: false
    file: ""  # If set, write JSONL to file
    console: false  # If true, write to stderr

  # Filtering
  filter:
    enabled: true
    respectDebugMode: true

    events:
      include: []  # Empty = all
      exclude:
        - "agent.user.prompt"

    attributes:
      redact:
        - "prompt"
        - "user.email"
        - "tool_output"
      hash:
        - "session_id"

    sampling:
      default: 1.0
      rates: {}

  # Resource attributes (added to all events)
  resource:
    service.name: "scion-agent"
    # Additional attributes from environment:
    # agent.id, grove.id, runtime.broker populated automatically
```

---

## 11. Open Questions

### 11.1 Cloud Backend Selection

**Decision:** Google Cloud Observability (Cloud Trace, Cloud Logging, Cloud Monitoring) is the primary target for the initial implementation.

**Options considered:**
1. **Google Cloud Observability** (Selected): Native GCP integration, unified with existing infra.
2. Generic OTLP Collector: Flexibility but higher operational overhead.
3. Honeycomb: Excellent UX but potential cost at scale.

**Impact:** Configuration and authentication will assume GCP-native identity (Workload Identity) or service account keys.

### 11.2 Prompt Logging Opt-In

**Decision:** Opt-in is managed at the **Grove** level by the grove administrator.

**Details:**
- Configured in the Grove settings on the Hub.
- When enabled, prompt and response logs are routed to a specific log destination (e.g., a restricted Cloud Logging bucket) to segregate sensitive content.

### 11.3 Cost Estimation Accuracy

**Decision:** Financial cost calculation is postponed. The system will track **token usage only** in the initial release.

**Rationale:**
- Pricing is complex and volatile.
- A future module may provide a price table function to convert token counts to approximate financial cost.

### 11.4 Session File Watching

**Decision:** **End-of-session parsing only** for Gemini CLI.

**Rationale:**
- Simpler implementation than real-time file watching.
- It is currently unclear if real-time session file parsing provides significant value over the OTel data stream.

### 11.5 Multi-Model Sessions

**Decision:** Metrics will be **broken down by model** within the session summary.

**Details:**
- The `agent_session_metrics` table and Hub API will support detailed breakdowns of token usage per model, rather than attributing everything to a single primary model.

### 11.6 Cross-Agent Correlation

**Decision:** Postponed.

**Details:**
- Initial implementation treats agents as independent.
- Future cross-agent correlation will likely be mediated by the Hub using shared identifiers when it orchestrates multi-agent workflows.

### 11.7 Retention and Archival

**Decision:** **Indefinite retention** of Hub-stored session summaries.

**Details:**
- The data volume for session summaries is low enough to retain indefinitely.
- Manual purge or cleanup scripts can be developed if storage becomes an issue.

### 11.8 Credential Injection for Agents

**Decision:** **Out of Scope**.

**Details:**
- We will assume that the key libraries will be able to load via the 'application default credentials' pattern.
- It will be up to the runtime broker design to ensure these are available to the sciontool environment.

### 11.9 Data Resiliency

**Decision:** **Configurable Flush Interval**.

**Details:**
- The flush interval will be made a configurable option with a sane default.
- Users who value metrics at the expense of load can choose a shorter interval to minimize data loss risk on crash.

### 11.10 Stdout/Stderr Handling

**Decision:** **Resolved**.

**Details:**
- This is now captured in Section 9.3 (OTel Log Bridge Architecture).

---

## 12. Engineering Milestones

### Milestone 1: Telemetry Foundation (Sciontool) ✅ COMPLETE

**Goal:** Enable `sciontool` to accept OTLP data and forward it to the Google Cloud backend.

**Status:** Completed 2026-02-05

**Deliverables:**
- [x] **OTLP Receiver**: Embedded receiver in `sciontool` listening on default ports (4317/4318).
- [x] **Cloud Forwarder**: Exporter for Google Cloud Trace/Monitoring/Logging.
- [x] **Configuration**: `telemetry` config block parsing and environment variable injection.
- [x] **Basic Filtering**: Implementation of include/exclude logic for event types.

**Test Criteria:**
- `sciontool` starts without errors with telemetry enabled.
- Can send dummy OTLP data (via `otel-cli` or similar) to localhost:4317.
- Dummy data appears in Google Cloud Console (Trace/Log Viewer).

#### Implementation Notes

**Package Structure:** `pkg/sciontool/telemetry/`

| File | Description |
|------|-------------|
| `config.go` | Configuration loading from env vars (SCION_TELEMETRY_*, SCION_OTEL_*) |
| `filter.go` | Include/exclude filtering with privacy default (agent.user.prompt excluded) |
| `exporter.go` | OTLP gRPC/HTTP exporter with raw proto forwarding |
| `receiver.go` | Embedded OTLP gRPC (4317) and HTTP (4318) receivers |
| `pipeline.go` | Main orchestration: Start/Stop lifecycle, span handler |
| `*_test.go` | Unit tests for config, filter, and pipeline |

**Key Design Decisions:**

1. **Environment-first configuration**: Follows `hub/client.go` pattern with `SCION_*` env vars.
2. **Non-blocking startup**: Telemetry failures log errors but don't block agent startup.
3. **Privacy default**: `agent.user.prompt` excluded by default.
4. **Raw proto forwarding**: Uses `ExportProtoSpans()` to forward OTLP data directly without SDK span conversion (avoids `ReadOnlySpan` private method constraint).
5. **Graceful shutdown**: 5-second timeout for telemetry flush on shutdown.

**Integration Point:** `cmd/sciontool/commands/init.go`
- Pipeline starts after `setupHostUser()` and before lifecycle hooks.
- Deferred shutdown ensures flush before container exit.

**Dependencies Added:**
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc`
- `go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp`
- `go.opentelemetry.io/proto/otlp` (upgraded to v1.9.0)

### Milestone 2: Harness Data & Log Bridge ✅ COMPLETE

**Goal:** Normalize data from harnesses and system components into the telemetry stream.

**Status:** Completed 2026-02-06

**Deliverables:**
- [x] **Hook Normalization**: TelemetryHandler converts harness hooks to OTLP spans with `agent.*` naming.
- [x] **Session Parsing**: Logic to parse Gemini CLI `session-*.json` files on session end.
- [x] **Log Bridge**: `otelslog` integration for Hub and Runtime Broker structured logging.
- [x] **Attribute Redaction**: Privacy filter implementation for sensitive fields (redact + hash).

**Test Criteria:**
- Run a Gemini agent session: tool calls appear as spans in GCP Trace.
- Agent logs (stdout/stderr) appear in GCP Logging with correct `agent_id` labels.
- Sensitive data (prompts) is redacted or absent based on config.

#### Implementation Notes

**Package Structure:**

| File | Description |
|------|-------------|
| `pkg/sciontool/hooks/handlers/telemetry.go` | TelemetryHandler converts hook events to OTLP spans |
| `pkg/sciontool/hooks/session/parser.go` | Parses Gemini `session-*.json` for token metrics |
| `pkg/sciontool/telemetry/filter.go` | Extended with `Redactor` for attribute redaction/hashing |
| `pkg/util/logging/otel.go` | Multi-handler and OTel bridge support |
| `pkg/util/logging/otel_provider.go` | LoggerProvider initialization for OTel log bridge |

**Hook-to-Span Mapping:**

| Hook Event | Span Name | Key Attributes |
|------------|-----------|----------------|
| `session-start` | `agent.session.start` | session_id, source |
| `session-end` | `agent.session.end` | session_id, reason, tokens_*, duration_ms |
| `tool-start` | `agent.tool.call` | tool_name, tool_input (redacted) |
| `tool-end` | `agent.tool.result` | tool_name, success, duration_ms |
| `prompt-submit` | `agent.user.prompt` | prompt (redacted) |
| `model-start` | `gen_ai.api.request` | model |
| `model-end` | `gen_ai.api.response` | success |

**Redaction Configuration:**

| Environment Variable | Default | Description |
|---------------------|---------|-------------|
| `SCION_TELEMETRY_REDACT` | `prompt,user.email,tool_output,tool_input` | Fields replaced with `[REDACTED]` |
| `SCION_TELEMETRY_HASH` | `session_id` | Fields replaced with SHA256 hash |

**Session Metrics Extraction:**

On `session-end` events, the TelemetryHandler parses the latest Gemini session file from `~/.gemini/sessions/` and adds these attributes to the span:
- `tokens_input`, `tokens_output`, `tokens_cached`
- `turn_count`, `duration_ms`, `model`
- Per-tool statistics: `tool.<name>.calls`, `tool.<name>.success`, `tool.<name>.errors`

**OTel Log Bridge Pattern:**

The Hub and Runtime Broker use a multi-handler approach:
1. Base handler (JSON or GCP-formatted) for local output
2. OTel bridge handler for forwarding to OTLP endpoint
3. Both handlers receive all log records simultaneously

Enabled via `SCION_OTEL_LOG_ENABLED=true` with endpoint in `SCION_OTEL_ENDPOINT`.

**Dependencies Added:**
- `go.opentelemetry.io/contrib/bridges/otelslog`
- `go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc`
- `go.opentelemetry.io/otel/sdk/log`

### Milestone 3: Hub Reporting & Storage

**Goal:** Aggregate session data and persist it to the Hub for state management.

**Deliverables:**
- [ ] **Aggregation Engine**: In-memory accumulation of session stats in `sciontool` (token counts, tool usage).
- [ ] **Hub Protocol**: Extension of daemon heartbeat/status updates to carry metrics payloads.
- [ ] **Hub Database**: Schema migration for the `agent_session_metrics` table.
- [ ] **Hub Ingestion**: Logic in Hub to receive metrics payloads and write to DB.

**Test Criteria:**
- Upon agent session completion, a row is created in `agent_session_metrics`.
- Token counts and tool usage statistics in the DB match the actual session activity.

### Milestone 4: Hub API & Web UI

**Goal:** Expose and visualize metrics in the user interface.

**Deliverables:**
- [ ] **Hub API**: Endpoints for retrieving session (`GET /metrics/session/{id}`) and agent summaries.
- [ ] **Web UI Component**: Session detail view showing token usage and cost estimates.
- [ ] **Web UI Dashboard**: Agent list view showing aggregate activity stats.

**Test Criteria:**
- Web UI "Session" tab displays correct token usage for a completed session.
- Agent list displays accurate "Total Tokens" or "Last Active" metrics.
