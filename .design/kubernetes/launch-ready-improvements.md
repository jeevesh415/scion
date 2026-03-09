# Kubernetes Runtime: Launch-Ready Improvements

## Purpose
This document reviews the **current Kubernetes runtime implementation in code** and proposes a staged plan to mature it toward launch readiness, with priority on parity with local runtimes (Docker, Apple container, Podman).

Scope of review is based on current implementation in:
- `pkg/runtime/k8s_runtime.go`
- `pkg/runtime/factory.go`
- `pkg/runtime/common.go`
- `pkg/runtime/docker.go`
- `pkg/runtime/podman.go`
- `pkg/runtime/apple_container.go`
- `pkg/agent/run.go`
- Kubernetes runtime tests in `pkg/runtime/k8s_*_test.go`

## Executive Summary
The Kubernetes runtime is functional for core agent lifecycle operations (run/list/logs/attach/sync/delete), has meaningful secret support (native Secret and GKE Secret Store CSI paths), and includes GCS volume support for GKE CSI.

However, compared to local runtimes, there are major parity and production-readiness gaps:
1. **Config parity gaps**: Kubernetes-specific config fields are partially ignored (`context`, `runtimeClassName`, and effectively `kubernetes.namespace` at agent level).
2. **Env/auth parity gaps**: Kubernetes does not reuse `buildCommonRunArgs`, so it misses harness env/telemetry env behavior and has divergent auth handling.
3. **Volume parity gaps**: non-GCS volume mounts from config are not implemented on Kubernetes.
4. **Namespace correctness gaps**: operations like delete/logs/attach/exec default to one namespace and can miss agents started elsewhere.
5. **Security/portability issues**: `ResolvedAuth` files use `hostPath` mounts, which are not portable to remote clusters and often disallowed.
6. **Operational maturity gaps**: limited retry/error classification, no structured diagnostics per pod lifecycle phase, and minimal e2e verification coverage.

## What Is Implemented Today

### Runtime selection and base config
- Runtime factory supports `kubernetes`/`k8s`, sets defaults (`DefaultNamespace`, `SyncMode=tar`), and reads runtime-level namespace/sync/GKE mode from settings.
- `context` is recognized in settings but not actually applied to client selection.

### Pod launch flow
- `Run()` creates pod, waits for ready state with status polling, and then performs initial home/workspace sync.
- Workspace and home paths are persisted in pod annotations for later sync operations.
- Tmux command wrapping is implemented in pod command construction.

### Secrets
- Fallback path: creates native Kubernetes Secret and injects environment/file/variable secrets.
- GKE path: creates SecretProviderClass and mounts CSI volume, with env refs from synced K8s Secret.
- Secret cleanup on delete is implemented (Secrets + SecretProviderClass when enabled).

### Sync modes
- `tar` snapshot sync to/from pod is implemented over `pods/exec` streaming.
- `mutagen` mode is implemented, with creation/recovery of workspace/home sessions.
- GCS volume sync metadata (`scion.gcs_volumes`) is supported for `sync to/from`.
<!-- Feedback - mutagen was never working and should be removed -->
### Runtime operations
- `List`, `GetLogs`, `Attach`, `Exec`, `Delete`, `Stop`, `GetWorkspacePath` exist and are wired.
- Attach includes terminal raw mode and resize support.

### Kubernetes resources
- Supports generic resource requests/limits from `ResourceSpec` and overlays Kubernetes-specific extended resources.
- Supports `ServiceAccountName`.

## Parity Comparison vs Local Runtimes

## 1) Environment and harness parity: **Partial / Divergent**
Local runtimes rely on `buildCommonRunArgs()` which applies:
- harness env (`Harness.GetEnv`)
- telemetry env (`Harness.GetTelemetryEnv` when enabled)
- resolved auth env/files
- common mount/env conventions (including optional local gcloud config)

Kubernetes runtime builds pod env independently and currently only ingests:
- `config.Env`
- secret-derived env
- some auth env (only in `ResolvedAuth` branch)
- host UID/GID env

Result: runtime behavior differs by backend for env/auth composition.

## 2) Auth parity: **Incomplete and risky**
- Kubernetes has `if len(ResolvedSecrets) > 0 { ... } else if ResolvedAuth != nil { ... }` behavior, so `ResolvedAuth` file/env injection is skipped whenever secrets are present.
- `ResolvedAuth.Files` use `hostPath` mounts. That is not a reliable mechanism for remote brokers/clusters and is often blocked by policy.

## 3) Volume parity: **Missing for non-GCS volumes**
- Local runtimes support generic local volume mounts.
- Kubernetes runtime only processes GCS volumes; non-GCS configured volumes are effectively ignored.

## 4) Namespace parity/correctness: **Weak**
- Namespace can be selected at run time from labels/settings, but several operations (`Delete`, `GetLogs`, `Attach`, `Exec`) default to `DefaultNamespace` without robust per-agent namespace resolution.
- `List` is single-namespace only.

This can cause lifecycle failures for agents created outside default namespace.

## 5) Kubernetes config parity: **Partial**
- `serviceAccountName` and resource overlays are used.
- `runtimeClassName` exists in API but is not applied to pod spec.
- runtime `context` is read but not applied to k8s client.
- agent-level `kubernetes.namespace` is not clearly honored in runtime namespace resolution path.

## 6) Local workspace parity: **Different model, acceptable but underdeveloped**
- Local runtimes use direct bind mounts for live local filesystem semantics.
- Kubernetes uses `EmptyDir` + sync (tar/mutagen), which is expected for remote clusters.
- Current sync model lacks stronger resumability/incrementality/observability expected for production parity.

## 7) Operational hardening parity: **Below local runtimes**
- Image handling is effectively a stub (`ImageExists=true`, `PullImage=nil`).
- Limited structured error taxonomy around scheduling/image/permission/network failures.
- BuildPod uses `resource.MustParse` which can panic on invalid values instead of returning user-friendly errors.

## 8) Test maturity: **Good unit coverage in key areas, limited integration/e2e**
Current tests cover:
- pod env basics
- tmux command wrapping
- annotation behavior
- secret creation/injection paths including GKE SPC

Gaps:
- namespace behavior across lifecycle operations
- auth file handling portability/security
- non-GCS volume behavior
- runtimeClassName/context wiring
- sync failure/retry edge cases
- multi-namespace listing/management

## Launch Risks (Current)
1. Agents in non-default namespaces can become hard to manage (`logs/attach/delete/exec` mismatch).
2. Remote cluster deployments may fail due to `hostPath` auth file assumptions.
3. Users can configure Kubernetes fields that are silently ignored, causing drift between config intent and runtime behavior.
4. Divergent env/auth semantics across runtimes create hard-to-debug behavior differences.
5. Invalid resource strings can crash runtime path due to `MustParse`.

## Staged Improvement Plan

## Stage 0: Correctness and Safety Baseline (Immediate)
Goal: remove sharp edges and stop silent misconfiguration.

Deliverables:
1. Namespace resolution hardening
- Persist namespace annotation on pod at create time (source of truth).
- Resolve namespace per-agent in `Delete`, `GetLogs`, `Attach`, `Exec`, `Sync`.
- Optional: allow `namespace/pod` ID format consistently across APIs.

2. Config wiring fixes
- Apply `kubernetes.runtimeClassName` to `pod.Spec.RuntimeClassName`.
- Implement runtime `context` selection in client construction (or fail clearly if unsupported).
- Honor agent-level `kubernetes.namespace` with clear precedence order:
  1) agent config `kubernetes.namespace`
  2) explicit runtime label override
  3) runtime default namespace from settings

3. Safe resource parsing
- Replace `resource.MustParse` with parse+validation returning actionable errors.

4. Auth correctness
- Remove `ResolvedSecrets` vs `ResolvedAuth` mutual exclusion; both pipelines should compose.

5. Compatibility guardrails
- If unsupported features are configured (until implemented), fail fast with explicit error rather than silently ignoring.

Acceptance checks:
- Unit tests for namespace-aware lifecycle operations.
- Tests validating runtimeClassName and context behavior.
- Tests ensuring invalid resource strings return errors (no panic).

## Stage 1: Runtime Parity Foundation (Near Term)
Goal: align Kubernetes behavior with local runtimes where functionally equivalent.

Deliverables:
1. Common env/auth composition parity
- Extract shared env/auth composition logic from `buildCommonRunArgs` into reusable helpers.
- Reuse helpers in Kubernetes `buildPod` to match local runtime behavior:
  - harness env
  - telemetry env
  - resolved auth env/files semantics

2. Non-GCS volume strategy
- Define Kubernetes-compatible handling for `type=local` volumes:
  - For broker-local clusters only: optional controlled `hostPath` mode behind explicit setting.
  - For remote/multi-node clusters: use PVC/ephemeral volume abstraction or explicit unsupported error.
- Keep GCS CSI path as first-class where configured.

3. Git/workspace parity behavior
- Validate `gitClone` mode behavior in Kubernetes start path and ensure command/workdir semantics match local runtimes.
- Ensure workspace path annotations and sync initialization stay consistent.

4. Observability parity
- Add structured runtime events/logs around each launch phase:
  - pod create
  - wait/schedule
  - image pull
  - home sync
  - workspace sync
- Include namespace/context in diagnostics.

Acceptance checks:
- Snapshot parity tests that compare computed env/auth across docker/podman/container/k8s for same RunConfig.
- Tests for local-volume behavior matrix (supported/unsupported with clear errors).

## Stage 2: Production Hardening (Mid Term)
Goal: make Kubernetes runtime resilient under real cluster conditions.

Deliverables:
1. Robust sync engine behavior
- Add retry/backoff and better error classification for tar/exec stream failures.
- Add checksum/incremental sync optimization for tar mode (or document limits clearly).
- Mutagen lifecycle reliability improvements (session recovery, stale session cleanup, explicit status reporting).

2. Pod spec hardening
- Security context defaults (runAsUser/fsGroup where compatible with image/user model).
- Optional node selectors/tolerations/affinity in Kubernetes config for scheduling control.
- Optional ephemeral-storage limits from common resource model.

3. Image handling policy
- Improve preflight and user feedback around image pull failures.
- Optional configurable pull policy.

4. Multi-namespace operations
- Configurable list scope:
  - single namespace (default)
  - explicit all managed namespaces (opt-in)

Acceptance checks:
- Integration tests against a real cluster profile (kind/k3d for CI, plus optional GKE lane).
- Failure-injection tests for pull failures, permission errors, and interrupted sync.

## Stage 3: Launch Readiness and UX (Final)
Goal: ship a predictable, documented, supportable Kubernetes runtime.

Deliverables:
1. CLI/user experience polish
- Improve user-facing error messages with direct remediation.
- Add `scion doctor` runtime checks for k8s prerequisites (context, namespace access, required CRDs/CSI drivers where used).

2. Documentation and policy
- Publish explicit support matrix:
  - supported volume types per deployment mode
  - secret modes (native vs GKE CSI)
  - sync modes and tradeoffs
  - required RBAC/pod security assumptions

3. Release gates
- Define launch SLOs and test gates:
  - agent start success rate
  - attach success rate
  - sync correctness/latency targets
  - cleanup success rate

Acceptance checks:
- End-to-end conformance checklist passes in CI and release candidates.
- Documentation matches implementation with no known silent no-op fields.

## Prioritized Backlog (Suggested)
1. Fix namespace resolution across all lifecycle methods.
2. Wire `runtimeClassName`, context support, and namespace precedence.
3. Remove `MustParse` panics.
4. Unify env/auth composition with shared helper logic.
5. Replace/contain `hostPath` auth file strategy.
6. Implement non-GCS volume behavior (or explicit unsupported errors by mode).
7. Add integration test lanes and failure-mode tests.
8. Add observability and doctor checks.

## Definition of “Parity Enough” for First Launch
Kubernetes runtime is launch-ready when:
1. Core lifecycle (`start/list/logs/attach/sync/exec/delete`) is namespace-correct and deterministic.
2. Env/auth behavior matches local runtimes for equivalent configs.
3. Unsupported Kubernetes constraints are explicit errors, not silent ignores.
4. Sync behavior is reliable with clear failure recovery paths.
5. CI includes real-cluster integration coverage for critical paths.
