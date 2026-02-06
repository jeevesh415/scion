---
title: Configuration Overview
---

Scion uses a multi-layered configuration system to manage orchestrator behavior, agent execution, and server operations.

## Configuration Domains

The documentation is divided into the following domains:

### [Orchestrator Settings (settings.yaml)](/reference/orchestrator-settings/)
Global and project-level settings for the `scion` CLI and orchestrator. Defines Runtimes, Harnesses, and execution Profiles.

### [Agent & Template Configuration (scion-agent.json)](/reference/agent-config/)
Configuration for agent blueprints (templates) and individual agent instances. Defines container images, volumes, and environment variables.

### [Server Configuration (Hub & Runtime Broker)](/reference/server-config/)
Operational settings for the Scion Hub and Runtime Broker services, including database and networking configuration.

### [Web Dashboard Configuration](/reference/web-config/)
Environment variables and settings for the Web Dashboard frontend and BFF.

### [Harness-Specific Settings](/reference/harness-settings/)
Guide to configuring the LLM tools and harnesses running *inside* the agent containers.

---

## Resolution Hierarchy

Scion typically resolves configuration using the following precedence (from highest to lowest):

1. **CLI Flags**: `--hub`, `--profile`, etc.
2. **Environment Variables**: `SCION_*` and `SCION_SERVER_*`.
3. **Grove Settings**: `.scion/settings.yaml` in the current project.
4. **Global Settings**: `~/.scion/settings.yaml` in the user's home directory.
5. **Defaults**: Hardcoded system defaults.
