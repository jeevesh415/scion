# Auth Propagation Design for Scion Agents

This document proposes a design for propagating authentication credentials from the host machine into container-based `scion` agents. The primary goal is to enable the `gemini` CLI and other tools running inside the agent to interact with Gemini and Vertex AI APIs using the user's existing host-side authentication.

## Background

`scion` agents run in isolated containers (Docker or Apple Virtualization Framework). To function effectively, they often need access to:
1.  **Gemini API Keys**: `GEMINI_API_KEY` or `GOOGLE_API_KEY`.
2.  **Vertex AI Auth**: `VERTEX_API_KEY` or Service Account keys.
3.  **GCP Project Context**: `GOOGLE_CLOUD_PROJECT` or `GCP_PROJECT`.
4.  **OAuth Credentials**: `~/.gemini/oauth_creds.json` when `selectedType` is `oauth-personal`.

The `gemini-cli` itself handles sandboxing by mounting host directories (like `~/.config/gcloud`) and selectively passing environment variables. `scion` should adopt a similar but more "agent-centric" approach.

## Proposed Mechanism

Authentication propagation will occur in two phases: **Discovery** (on the host) and **Injection** (into the container).

### 1. Discovery (Host-side)

The `scion` CLI (running on the host) will look for authentication markers in the following order of precedence:

1.  **Explicit Environment Variables**:
    - `GEMINI_API_KEY`
    - `GOOGLE_API_KEY`
    - `VERTEX_API_KEY`
    - `GOOGLE_APPLICATION_CREDENTIALS`
    - `GOOGLE_CLOUD_PROJECT` / `GCP_PROJECT`
2.  **Global Settings**:
    - Reading `~/.gemini/settings.json` to extract keys if not present in the environment.
    - Detecting if `selectedType` is `oauth-personal` and finding `oauth_creds.json`.

### 2. Injection (Runtime-side)

Once discovered, `scion` will inject these credentials into the agent runtime.

#### A. Environment Variable Propagation
For API keys, the simplest and most secure method for containers is environment variable injection.
- The `RunConfig` in `pkg/runtime` will be updated to automatically include discovered auth environment variables.
- Runtimes (Docker/Apple) will append these to the `run` command (e.g., `-e GEMINI_API_KEY=...`).
- `GEMINI_DEFAULT_AUTH_TYPE` will be set based on the discovered auth method.

#### B. Filesystem Mirroring (ADC and OAuth)
If `GOOGLE_APPLICATION_CREDENTIALS` is set and points to a file, `scion` should:
1.  Mount the credential JSON file into a standard location in the container (e.g., `/home/node/.config/gcp/application_default_credentials.json`).
2.  Set the `GOOGLE_APPLICATION_CREDENTIALS` environment variable inside the container to point to this internal path.

If OAuth is detected:
1.  Mount `~/.gemini/oauth_creds.json` to `/home/node/.gemini/oauth_creds.json` in the container.
2.  Set `GEMINI_DEFAULT_AUTH_TYPE=oauth-personal` inside the container.


### 3. Agent Configuration Alignment

Each agent has its own `.gemini/settings.json` in its home directory. 
- During `scion grove init`, the template should *not* contain hardcoded keys.
- Instead, the agent's system prompt or configuration should be designed to rely on the environment variables provided by the runtime.

## Security Considerations

1.  **Key Exposure**: Environment variables injected via `docker run -e` are visible in `docker inspect` and to other processes inside the container. Since agents are isolated and controlled by the user, this is generally acceptable for a local dev tool, but should be documented.
2.  **Read-Only Mounts**: If mounting credential files, they should always be mounted as `:ro` (read-only).
3.  **Host-Path Leakage**: We must ensure that host-specific paths in variables like `GOOGLE_APPLICATION_CREDENTIALS` are translated to container-internal paths before injection.

## Implementation Steps

1.  **Update `pkg/config`**: Add logic to gather auth environment variables from the host.
2.  **Update `pkg/runtime`**:
    - Expand `RunConfig` to include a dedicated `Auth` field or ensure `Env` is populated with discovered keys.
    - Implement path translation for `GOOGLE_APPLICATION_CREDENTIALS`.
3.  **Refactor `DockerRuntime` and `AppleContainerRuntime`**: Ensure they process the new auth-related configuration.
4.  **CLI Flag**: Add a `--no-auth-propagation` flag to `scion start` for users who want to keep an agent entirely unauthenticated or manually provide keys.
