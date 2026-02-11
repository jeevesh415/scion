# Secrets Management Design

**Status:** Draft
**Updated:** 2026-02-11

## 1. Overview

This document specifies the design for secrets management in Scion. Secrets are sensitive values—API keys, credentials, certificates, configuration files—that agents need at runtime. They are stored centrally in the Hub (or an external secrets backend) and projected into agent containers at provisioning time.

The current system provides basic environment-variable-style secrets (key-value pairs injected as `ResolvedEnv` in the `CreateAgent` dispatch). This design extends secrets into a first-class concept with a typed interface, pluggable storage backends, and runtime-specific projection strategies.

### 1.1 Goals

- **Typed secrets**: Distinguish between environment variables, opaque variables (non-env key-value), and file-system secrets.
- **Pluggable storage**: Define an abstract storage interface with a primary GCP Secret Manager implementation.
- **Runtime-aware projection**: Each runtime (Docker, Apple, Kubernetes, Cloud Run) projects secrets using its native capabilities.
- **Least-privilege**: Secrets are write-only in the Hub API, decrypted only at provisioning time, and scoped to the narrowest context required.
- **Backward compatibility**: The existing `EnvVar` and `Secret` store models and API endpoints continue to work. The new typed secret model is layered alongside them.

### 1.2 Non-Goals (This Iteration)

- Secret rotation policies and automated key cycling.
- Agent-initiated secret access at runtime (secrets are injected at start, not fetched on demand).
- Audit logging of secret access events (tracked as a future enhancement).
- Multi-cloud secret backends (AWS Secrets Manager, HashiCorp Vault) beyond GCP.

---

## 2. Secret Model

### 2.1 Secret Types

A secret has a **type** that determines how it is projected into the agent container:

| Type | Description | Target Semantics |
|------|-------------|------------------|
| `environment` | Injected as a container environment variable | Target is the environment variable name (e.g., `ANTHROPIC_API_KEY`) |
| `variable` | An opaque key-value pair stored but not automatically injected as an env var | Target is a logical name; consumed by templates or tooling |
| `file` | Projected as a file on the container filesystem | Target is the absolute file path (e.g., `/home/scion/.config/credentials.json`) |

### 2.2 Go Interface Definition

```go
package secret

// Type represents the kind of secret and how it is projected.
type Type string

const (
    TypeEnvironment Type = "environment"
    TypeVariable    Type = "variable"
    TypeFile        Type = "file"
)

// Secret is the core interface for a typed secret.
type Secret interface {
    // Name returns the unique identifier for this secret within its scope.
    Name() string

    // Type returns the secret type (environment, variable, or file).
    Type() Type

    // Target returns the projection target.
    // For environment secrets: the environment variable name.
    // For variable secrets: a logical key name.
    // For file secrets: the absolute container file path.
    Target() string

    // Value returns the secret's plaintext value as bytes.
    // For environment and variable secrets, this is the UTF-8 string value.
    // For file secrets, this is the raw file content.
    Value() []byte

    // Scope returns the scope at which this secret is defined.
    Scope() Scope

    // ScopeID returns the ID of the scoped entity (user ID, grove ID, or broker ID).
    ScopeID() string

    // Version returns the secret version, incremented on each update.
    Version() int

    // Description returns an optional human-readable description.
    Description() string
}

// Scope identifies the level at which a secret is defined.
type Scope string

const (
    ScopeUser          Scope = "user"
    ScopeGrove         Scope = "grove"
    ScopeRuntimeBroker Scope = "runtime_broker"
)
```

### 2.3 Concrete Implementation

```go
// SecretEntry is the standard implementation of the Secret interface.
type SecretEntry struct {
    name        string
    secretType  Type
    target      string
    value       []byte
    scope       Scope
    scopeID     string
    version     int
    description string
}

// NewEnvironmentSecret creates a secret projected as an environment variable.
func NewEnvironmentSecret(name, envKey string, value []byte, scope Scope, scopeID string) *SecretEntry {
    return &SecretEntry{
        name:       name,
        secretType: TypeEnvironment,
        target:     envKey,
        value:      value,
        scope:      scope,
        scopeID:    scopeID,
    }
}

// NewFileSecret creates a secret projected as a file.
func NewFileSecret(name, filePath string, content []byte, scope Scope, scopeID string) *SecretEntry {
    return &SecretEntry{
        name:       name,
        secretType: TypeFile,
        target:     filePath,
        value:      content,
        scope:      scope,
        scopeID:    scopeID,
    }
}
```

### 2.4 Store Model Extension

The existing `store.Secret` model is extended with type and target fields:

```go
// In pkg/store/models.go

type Secret struct {
    ID             string    `json:"id"`
    Key            string    `json:"key"`
    EncryptedValue string    `json:"-"`

    // NEW: Secret type and target
    SecretType string `json:"secretType"` // "environment", "variable", "file"
    Target     string `json:"target"`     // env var name, logical key, or file path

    Scope       string `json:"scope"`
    ScopeID     string `json:"scopeId"`
    Description string `json:"description,omitempty"`
    Version     int    `json:"version"`

    Created   time.Time `json:"created"`
    Updated   time.Time `json:"updated"`
    CreatedBy string    `json:"createdBy,omitempty"`
    UpdatedBy string    `json:"updatedBy,omitempty"`
}
```

Default behavior: if `SecretType` is empty, the secret is treated as `TypeEnvironment` with `Target` defaulting to `Key`. This preserves backward compatibility with existing secrets.

---

## 3. Storage Interface

### 3.1 Abstract Interface

```go
package secret

import "context"

// Filter specifies criteria for listing secrets.
type Filter struct {
    Scope   Scope  // Required
    ScopeID string // Required
    Type    Type   // Optional: filter by secret type
    Name    string // Optional: filter by exact name
}

// Store defines the abstract interface for secret storage backends.
type Store interface {
    // Get retrieves a secret by name within a scope.
    // Returns the secret with its decrypted value.
    Get(ctx context.Context, name string, scope Scope, scopeID string) (Secret, error)

    // Set creates or updates a secret.
    // The implementation is responsible for encrypting the value at rest.
    Set(ctx context.Context, s Secret) error

    // Delete removes a secret by name within a scope.
    Delete(ctx context.Context, name string, scope Scope, scopeID string) error

    // List returns secret metadata matching the filter.
    // Values are NOT populated in the returned secrets.
    List(ctx context.Context, filter Filter) ([]Secret, error)

    // Resolve returns all secrets applicable to a given agent context,
    // merging across scopes with the standard precedence:
    // user < grove < runtime_broker.
    // Returns secrets with their decrypted values.
    Resolve(ctx context.Context, userID, groveID, brokerID string) ([]Secret, error)
}
```

### 3.2 Resolution Semantics

The `Resolve` method implements the same hierarchical merge used for environment variables today:

1. **User scope** (lowest priority): Secrets defined for the agent's owner.
2. **Grove scope**: Secrets defined for the project.
3. **Runtime Broker scope** (highest priority): Secrets specific to the execution host.

Within the same scope, secrets with the same `Name` are deduplicated (last write wins). Across scopes, higher-priority scopes override lower ones when names collide.

### 3.3 Relationship to Existing Stores

The `secret.Store` interface is distinct from `store.SecretStore`. The latter is the low-level database persistence layer (SQLite/Postgres). The former is the higher-level abstraction that may delegate to `store.SecretStore`, GCP Secret Manager, or other backends.

```
                          ┌──────────────┐
                          │ secret.Store │  (business logic interface)
                          └──────┬───────┘
                                 │
                 ┌───────────────┼───────────────┐
                 │               │               │
         ┌───────────┐   ┌─────────────┐   ┌──────────┐
         │ SQLite     │   │ GCP Secret  │   │ Vault    │
         │ (existing) │   │ Manager     │   │ (future) │
         └───────────┘   └─────────────┘   └──────────┘
```

---

## 4. GCP Secret Manager Implementation

### 4.1 Overview

The primary production implementation uses [Google Cloud Secret Manager](https://cloud.google.com/secret-manager) for encrypted secret storage. This provides:

- Envelope encryption with Google-managed keys (or customer-managed via Cloud KMS).
- Automatic versioning of secret values.
- IAM-based access control.
- Audit logging via Cloud Audit Logs.

### 4.2 Secret Naming Convention

GCP Secret Manager has a flat namespace per project. Scion secrets are mapped using a hierarchical naming convention:

```
scion/{scope}/{scopeID}/{name}
```

Examples:
- `scion/user/user-abc/ANTHROPIC_API_KEY`
- `scion/grove/grove-xyz/DB_PASSWORD`
- `scion/runtime_broker/broker-123/TLS_CERT`

### 4.3 Implementation Sketch

```go
package gcpsecrets

import (
    "context"
    "fmt"

    secretmanager "cloud.google.com/go/secretmanager/apiv1"
    smpb "cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"

    "github.com/ptone/scion-agent/pkg/secret"
)

// GCPStore implements secret.Store using GCP Secret Manager.
type GCPStore struct {
    client    *secretmanager.Client
    projectID string
}

func NewGCPStore(ctx context.Context, projectID string) (*GCPStore, error) {
    client, err := secretmanager.NewClient(ctx)
    if err != nil {
        return nil, fmt.Errorf("create secret manager client: %w", err)
    }
    return &GCPStore{client: client, projectID: projectID}, nil
}

func (s *GCPStore) secretPath(name string, scope secret.Scope, scopeID string) string {
    secretName := fmt.Sprintf("scion-%s-%s-%s", scope, scopeID, name)
    return fmt.Sprintf("projects/%s/secrets/%s", s.projectID, secretName)
}

func (s *GCPStore) Get(ctx context.Context, name string, scope secret.Scope, scopeID string) (secret.Secret, error) {
    path := s.secretPath(name, scope, scopeID) + "/versions/latest"
    result, err := s.client.AccessSecretVersion(ctx, &smpb.AccessSecretVersionRequest{
        Name: path,
    })
    if err != nil {
        return nil, fmt.Errorf("access secret %s: %w", name, err)
    }
    // Retrieve metadata to determine type/target (stored as labels on the secret)
    // ...
    return secret.NewEnvironmentSecret(name, name, result.Payload.Data, scope, scopeID), nil
}

// Set, Delete, List, Resolve methods follow similar patterns...
```

### 4.4 Metadata Storage

GCP Secret Manager supports labels on secret resources. Scion stores the following metadata as labels:

| Label Key | Value | Purpose |
|-----------|-------|---------|
| `scion-scope` | `user`, `grove`, `runtime_broker` | Scope identification |
| `scion-scope-id` | UUID | Scoped entity ID |
| `scion-type` | `environment`, `variable`, `file` | Secret type |
| `scion-target` | URL-encoded target | Projection target |

### 4.5 Hybrid Storage Option

For deployments that prefer not to use GCP Secret Manager for all secrets, a hybrid approach is possible:

- **Secret metadata** (name, type, target, scope, version) stored in the Hub database.
- **Secret values** stored in GCP Secret Manager, referenced by a `secretRef` field.
- The `secret.Store` implementation joins metadata from the database with values from GCP SM.

This keeps the Hub database as the metadata authority while delegating value encryption to GCP.

---

## 5. Runtime Projection

Each runtime projects secrets differently based on its capabilities. The projection logic runs during agent provisioning, after the Hub resolves secrets and dispatches the `CreateAgent` command.

### 5.1 Resolved Secrets in CreateAgent

The `CreateAgentRequest` type is extended to include typed secrets alongside the existing `ResolvedEnv`:

```go
// In pkg/runtimebroker/types.go

type CreateAgentRequest struct {
    // ... existing fields ...

    ResolvedEnv map[string]string `json:"resolvedEnv,omitempty"`

    // NEW: Typed secrets resolved by the Hub.
    // Includes environment, variable, and file secrets.
    ResolvedSecrets []ResolvedSecret `json:"resolvedSecrets,omitempty"`
}

// ResolvedSecret is a secret resolved by the Hub for runtime projection.
type ResolvedSecret struct {
    Name   string `json:"name"`
    Type   string `json:"type"`   // "environment", "variable", "file"
    Target string `json:"target"` // env var name or file path
    Value  string `json:"value"`  // plaintext value (base64-encoded for file type)
    Source string `json:"source"` // scope that provided this secret (for diagnostics)
}
```

### 5.2 Docker Runtime

Docker is the most straightforward runtime for secret projection.

#### Environment Secrets
Passed as `-e` flags on the `docker run` command line, merged into the existing environment injection in `buildCommonRunArgs()`:

```go
for _, s := range resolvedSecrets {
    if s.Type == "environment" {
        addArg("-e", fmt.Sprintf("%s=%s", s.Target, s.Value))
    }
}
```

#### File Secrets
Written to the agent's provisioning home directory before container start. The home directory is already bind-mounted into the container, so files placed there are immediately available:

```go
for _, s := range resolvedSecrets {
    if s.Type == "file" {
        // Write the secret file to the agent's home directory tree
        hostPath := filepath.Join(homeDir, s.Target)
        os.MkdirAll(filepath.Dir(hostPath), 0700)
        os.WriteFile(hostPath, []byte(s.Value), 0600)
    }
}
```

If the target path is absolute and outside the home directory, an alternative is to use Docker's `--mount type=tmpfs` for a scratch area, but the simpler approach is to constrain file secret targets to paths relative to the container home or use additional bind mounts.

#### Variable Secrets
Variable-type secrets are not automatically injected. They are stored in a metadata file within the home directory that tooling (e.g., `sciontool`) can read:

```
/home/scion/.scion/secrets.json
```

### 5.3 Apple Container Runtime

The Apple Virtualization Framework (`container` CLI) supports bind mounts of directories but has limited support for individual file mounts. This creates challenges for file-type secrets.

#### Environment Secrets
Same as Docker—passed via `-e` flags. The Apple runtime reuses `buildCommonRunArgs()`.

#### File Secrets — Proposed Approaches

| Approach | Description | Pros | Cons |
|----------|-------------|------|------|
| **A. Home directory hydration** (Recommended) | Write secret files into the agent's provisioning home directory before container start. Since the home dir is bind-mounted, files appear inside the container. | Simple, consistent with Docker. No runtime-specific logic needed. | Target path must be within the home directory mount. Secrets persist on the host filesystem. |
| **B. Init script injection** | Write secrets to a staging directory. Add an init script that copies them to their target paths on container start. | Supports arbitrary target paths. | Adds complexity. Requires modifying the container entrypoint. Race condition between init and agent start. |
| **C. Directory mount with symlinks** | Mount a secrets directory into the container. Create symlinks from target paths to the mounted files. | Clean separation. Secrets in a single known location. | Symlink creation requires init logic. Apple container may not support all symlink scenarios. |
| **D. `container exec` post-start** | After the container starts, use `container exec` to write secret files into the running container. | Supports arbitrary paths. No entrypoint modification. | Race condition with agent process. Requires container to be running. Additional exec overhead. |

**Recommendation:** Approach A (home directory hydration) for the initial implementation. This works identically to Docker since both runtimes mount the agent home directory. For targets outside the home directory, fall back to Approach D (`container exec` post-start) with a short delay to ensure the container is ready.

#### Variable Secrets
Same as Docker—written to `~/.scion/secrets.json`.

### 5.4 Kubernetes Runtime

Kubernetes has native support for both environment variable and file-based secret projection via the Kubernetes Secrets API.

#### GCP Secret Manager Integration

For GCP-hosted Kubernetes clusters (GKE), secrets should be projected using **GCP Secret Manager CSI Driver** or **Workload Identity** rather than duplicating values into Kubernetes Secret objects. This avoids storing plaintext values in etcd.

##### Option 1: SecretProviderClass (CSI Driver)

```yaml
apiVersion: secrets-store.csi.x-k8s.io/v1
kind: SecretProviderClass
metadata:
  name: agent-secrets-<agentId>
spec:
  provider: gcp
  parameters:
    secrets: |
      - resourceName: "projects/<project>/secrets/scion-user-<userId>-API_KEY/versions/latest"
        path: "api-key"
      - resourceName: "projects/<project>/secrets/scion-grove-<groveId>-TLS_CERT/versions/latest"
        path: "tls-cert"
```

Mounted as a volume in the agent Pod:

```yaml
volumes:
  - name: secrets
    csi:
      driver: secrets-store.csi.x-k8s.io
      readOnly: true
      volumeAttributes:
        secretProviderClass: agent-secrets-<agentId>
```

##### Option 2: External Secrets Operator

For non-GKE clusters, the [External Secrets Operator](https://external-secrets.io/) can sync GCP Secret Manager secrets into Kubernetes Secret objects:

```yaml
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: agent-<agentId>
spec:
  refreshInterval: 0  # One-time fetch
  secretStoreRef:
    name: gcp-secret-store
    kind: ClusterSecretStore
  target:
    name: agent-<agentId>-secrets
  data:
    - secretKey: ANTHROPIC_API_KEY
      remoteRef:
        key: scion-user-<userId>-ANTHROPIC_API_KEY
```

#### Environment Secrets
Projected via `envFrom` or individual `env` entries in the Pod spec referencing the Kubernetes Secret:

```yaml
env:
  - name: ANTHROPIC_API_KEY
    valueFrom:
      secretKeyRef:
        name: agent-<agentId>-secrets
        key: ANTHROPIC_API_KEY
```

#### File Secrets
Mounted as volumes from the Kubernetes Secret or CSI driver:

```yaml
volumeMounts:
  - name: secrets
    mountPath: /home/scion/.config/credentials.json
    subPath: credentials.json
    readOnly: true
```

### 5.5 Cloud Run (Future)

Cloud Run supports native GCP Secret Manager integration:

```yaml
env:
  - name: API_KEY
    valueFrom:
      secretKeyRef:
        secret: scion-user-abc-API_KEY
        version: latest
volumes:
  - name: secrets
    secret:
      secret: scion-grove-xyz-TLS_CERT
      items:
        - path: tls-cert.pem
          version: latest
```

This is the cleanest integration since Cloud Run natively resolves GCP Secret Manager references without any CSI driver or operator.

### 5.6 Projection Summary

| Capability | Docker | Apple | Kubernetes | Cloud Run |
|------------|--------|-------|------------|-----------|
| **Env secrets** | `-e` flag | `-e` flag | `envFrom` / `env.valueFrom` | `env.valueFrom` |
| **File secrets** | Home dir hydration | Home dir hydration | Volume mount / CSI | Volume mount |
| **Variable secrets** | `secrets.json` | `secrets.json` | ConfigMap or `secrets.json` | `secrets.json` |
| **GCP SM native** | No (values passed) | No (values passed) | Yes (CSI / ESO) | Yes (native) |
| **Secret in etcd/disk** | Host filesystem | Host filesystem | Optional (CSI avoids) | Never |

---

## 6. Hub API Changes

### 6.1 Extended Secret Endpoints

The existing secret API endpoints (Section 7.3 of `hub-api.md`) are extended to support the new type and target fields.

#### Set Secret (Updated)

```
PUT /api/v1/secrets/{key}
```

**Request Body:**
```json
{
  "value": "string",
  "scope": "user",
  "scopeId": "string",
  "description": "string",
  "type": "environment",
  "target": "ANTHROPIC_API_KEY"
}
```

New fields:
- `type` (optional, default: `"environment"`): One of `environment`, `variable`, `file`.
- `target` (optional, defaults to `key`): The projection target.

For `file` type secrets, `value` should be base64-encoded.

#### Get Secret Metadata (Updated)

```json
{
  "id": "uuid",
  "key": "my-api-key",
  "type": "environment",
  "target": "ANTHROPIC_API_KEY",
  "scope": "user",
  "scopeId": "user-abc",
  "description": "Anthropic API key for Claude agents",
  "version": 3,
  "created": "2026-01-24T10:00:00Z",
  "updated": "2026-02-11T14:30:00Z"
}
```

### 6.2 Resolved Secrets Endpoint (Internal)

A new internal endpoint for the Hub to resolve typed secrets during agent creation:

```
GET /api/v1/agents/{agentId}/resolved-secrets
```

**Response:**
```json
{
  "secrets": [
    {
      "name": "anthropic-key",
      "type": "environment",
      "target": "ANTHROPIC_API_KEY",
      "value": "sk-...",
      "source": "user"
    },
    {
      "name": "gcp-credentials",
      "type": "file",
      "target": "/home/scion/.config/gcloud/credentials.json",
      "value": "base64-encoded-content",
      "source": "grove"
    }
  ]
}
```

### 6.3 CLI Changes

The `scion hub secret set` command is extended:

```bash
# Environment secret (default, backward compatible)
scion hub secret set API_KEY sk-ant-...

# Explicit type
scion hub secret set --type=environment --target=ANTHROPIC_API_KEY api-key sk-ant-...

# File secret
scion hub secret set --type=file --target=/home/scion/.config/creds.json gcp-creds @./service-account.json

# Variable secret
scion hub secret set --type=variable config-value '{"setting": true}'
```

The `@` prefix for values reads from a local file (similar to `curl -d @file`).

---

## 7. Security Considerations

### 7.1 Value Transmission

- Secret values are transmitted over TLS between Hub and Runtime Brokers.
- For Docker and Apple runtimes, decrypted values traverse the control channel (WebSocket over TLS) and are present on the broker host filesystem (in the home directory) and in the container process environment.
- For Kubernetes and Cloud Run, the GCP Secret Manager CSI driver or native integration avoids transmitting plaintext values through the Hub at all—only secret references are passed.

### 7.2 Value at Rest

| Component | Encryption | Notes |
|-----------|-----------|-------|
| Hub database | Application-level encryption (existing) | `EncryptedValue` field, encrypted before storage |
| GCP Secret Manager | Envelope encryption (Google KMS) | Automatic, configurable CMEK |
| Docker host filesystem | Dependent on host disk encryption | Files in agent home directory |
| Kubernetes etcd | K8s encryption-at-rest config | Avoidable with CSI driver |

### 7.3 Value Lifecycle

1. **Write**: User sets secret via CLI/API → Hub encrypts and stores.
2. **Resolve**: Hub dispatches agent creation → decrypts and includes in `ResolvedSecrets`.
3. **Project**: Runtime Broker projects secrets into the container (env, file, etc.).
4. **Cleanup**: On agent deletion, broker removes provisioning directory (including secret files).

### 7.4 Logging

- Secret values MUST NOT appear in logs at any tier (Hub, Broker, Agent).
- The `ResolvedSecrets` field should be redacted in request/response logging.
- File contents for file-type secrets should never be logged.

---

## 8. Migration Path

### 8.1 Phase 1: Type-Aware Store Model

1. Add `SecretType` and `Target` columns to the secrets table (nullable, defaulting to `"environment"` and `Key` respectively).
2. Update `store.SecretStore` interface and SQLite implementation.
3. Update Hub API handlers to accept and return the new fields.
4. Update CLI `hub secret set/get` commands.

### 8.2 Phase 2: Runtime Projection

1. Add `ResolvedSecrets` to `CreateAgentRequest`.
2. Implement projection logic in the Runtime Broker's `CreateAgent` handler.
3. Docker: env injection and home directory file hydration.
4. Apple: same as Docker (home directory hydration).

### 8.3 Phase 3: GCP Secret Manager Backend

1. Implement `secret.Store` backed by GCP Secret Manager.
2. Configuration to select backend (SQLite-encrypted vs GCP SM).
3. Migration tooling to move existing secrets from SQLite to GCP SM.

### 8.4 Phase 4: Native K8s/Cloud Run Integration

1. K8s runtime generates `SecretProviderClass` or `ExternalSecret` resources.
2. Cloud Run runtime uses native secret references in service config.
3. These runtimes receive secret *references* rather than plaintext values from the Hub.

---

## 9. Open Questions

### 9.1 Secret Reference vs. Value in Dispatch

**Question:** Should the Hub always send plaintext values to the Broker, or should it send GCP Secret Manager references that the Broker resolves locally?

- **Plaintext dispatch** (current design for Docker/Apple): Simple, runtime-agnostic. The Hub resolves everything.
- **Reference dispatch** (possible for K8s/Cloud Run): More secure—the Broker or runtime resolves the reference directly from GCP SM. But requires the Broker (or the K8s service account) to have GCP SM read access.

**Proposal:** Use plaintext dispatch for Docker and Apple. Use reference dispatch for Kubernetes and Cloud Run where native integration is available. The `ResolvedSecret` type could include an optional `ref` field:

```go
type ResolvedSecret struct {
    // ... existing fields ...
    Ref string `json:"ref,omitempty"` // e.g., "projects/my-proj/secrets/scion-user-abc-API_KEY/versions/latest"
}
```

### 9.2 File Secret Size Limits

**Question:** What is the maximum size for file-type secrets?

GCP Secret Manager has a 64 KiB per-version limit. This is sufficient for certificates, credential files, and small config files but not for large binary blobs. Should Scion enforce a similar limit, or support larger files via GCS-backed storage?

**Proposal:** Enforce a 64 KiB limit for file secrets to match GCP SM limits. Larger files should use the existing template/workspace mechanisms.

### 9.3 Apple Container File Mounts

**Question:** What is the best long-term approach for file secrets on Apple containers?

The home directory hydration approach (5.3, Approach A) works for most cases but constrains target paths to the home directory. If Apple's `container` CLI adds individual file mount support in the future, the projection logic should be updated.

**Monitoring:** Track Apple container runtime releases for file mount support. In the interim, document the target path constraint for Apple users.

### 9.4 Secret Scope for Templates

**Question:** Should templates be able to declare "required secrets" that must be present for an agent to start?

For example, a Claude template could declare that `ANTHROPIC_API_KEY` is required. During agent creation, the Hub would verify that the secret exists in one of the applicable scopes and fail fast with a clear error if it's missing.

**Proposal:** Add an optional `requiredSecrets` field to `TemplateConfig`:

```go
type TemplateConfig struct {
    // ... existing fields ...
    RequiredSecrets []RequiredSecret `json:"requiredSecrets,omitempty"`
}

type RequiredSecret struct {
    Name string `json:"name"`           // Secret name to look for
    Type string `json:"type,omitempty"` // Expected type (optional)
}
```

### 9.5 Env Var / Secret Unification

**Question:** The current system has separate `EnvVar` and `Secret` models with overlapping capabilities (env vars can have `Secret: true`). Should these be unified?

The `EnvVar` model supports a `Secret` bool flag and an `InjectionMode` field. The `Secret` model is write-only with encrypted storage. With the introduction of typed secrets, there's an opportunity to consolidate.

**Proposal:** Defer unification. The existing separation works and avoids a risky migration. The new typed secret model builds on the `Secret` store, not `EnvVar`. The `EnvVar.Secret` flag continues to work as a convenience for simple cases where users want to set a secret env var without thinking about the typed system.

### 9.6 Secret Versioning and Rollback

**Question:** Should Scion support accessing specific secret versions or rolling back?

GCP Secret Manager natively supports versioning. The current design always uses `latest`. Should the `Resolve` endpoint support pinning a version?

**Proposal:** Defer. Always use latest for now. Version pinning adds complexity to the resolution logic and agent creation flow. If needed, GCP SM's native versioning can be exposed later.

### 9.7 Cross-Grove Secret Sharing

**Question:** Can secrets be shared across groves without duplication?

Currently, secrets are scoped to a single grove. A user who works on multiple groves with the same API key must set it in each grove or use user-scope secrets.

**Proposal:** User-scope secrets already solve this. Document that user-scope is the recommended approach for cross-grove secrets. Organization/team-scope secrets could be added later as part of the groups/permissions system.

---

## 10. References

- **Hosted Architecture:** `.design/hosted/hosted-architecture.md` Section 4.4
- **Hub API:** `.design/hosted/hub-api.md` Section 7
- **Runtime Broker API:** `.design/hosted/runtime-broker-api.md` Section 5
- **GCP Secret Manager:** https://cloud.google.com/secret-manager/docs
- **K8s Secrets Store CSI Driver:** https://secrets-store-csi-driver.sigs.k8s.io/
- **External Secrets Operator:** https://external-secrets.io/
