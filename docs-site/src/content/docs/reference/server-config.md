---
title: Server Configuration (Hub & Runtime Broker)
---

This document describes the configuration for the Scion Hub (State Server) and the Scion Runtime Broker services.

## Purpose
Server configuration controls the operational behavior of the Scion backend components in a "Hosted" or distributed architecture. This includes network settings, database connections, and security configurations.

## Locations
- **Config File**: `~/.scion/server.yaml` or `./server.yaml` in the current working directory.
- **Environment Variables**: Overridden using the `SCION_SERVER_` prefix (e.g., `SCION_SERVER_HUB_PORT`).

## Configuration Sections

### Hub Section (`hub`)
Configuration for the central Hub API server.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `port` | int | `9810` | The HTTP port to listen on. |
| `host` | string | `0.0.0.0` | The network address to bind to. |
| `readTimeout` | duration | `30s` | Maximum duration for reading the entire request. |
| `writeTimeout` | duration | `60s` | Maximum duration before timing out writes. |
| `corsEnabled` | bool | `true` | Whether to enable Cross-Origin Resource Sharing. |
| `corsAllowedOrigins` | list | `["*"]` | List of origins allowed to make CORS requests. |
| `corsAllowedMethods` | list | `[...]` | Standard HTTP methods allowed (GET, POST, etc). |
| `corsAllowedHeaders` | list | `[...]` | Allowed headers including Scion-specific tokens. |
| `corsMaxAge` | int | `3600` | How long the results of a preflight request can be cached. |

### RuntimeBroker Section (`runtimeBroker`)
Configuration for the Runtime Broker service.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `enabled` | bool | `false` | Whether to start the Runtime Broker API. |
| `port` | int | `9800` | The HTTP port to listen on. |
| `host` | string | `0.0.0.0` | The network address to bind to. |
| `mode` | string | `connected` | Operational mode (currently only `connected` supported). |
| `hubEndpoint` | string | | The Hub API endpoint for status reporting. |
| `brokerId` | string | (auto) | Unique identifier for this broker (persisted in settings). |
| `brokerName` | string | | Human-readable name for this runtime broker. |

### Database Section (`database`)
Persistence settings for the Hub.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `driver` | string | `sqlite` | Database driver (`sqlite` or `postgres`). |
| `url` | string | `hub.db` | Connection path for SQLite or DSN for PostgreSQL. |

### Auth Section (`auth`)
Settings for development and domain authorization.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `devMode` | bool | `false` | Enable development token authentication. **Not for production.** |
| `devToken` | string | | An explicitly configured development token. |
| `devTokenFile` | string | `~/.scion/dev-token` | Path to the auto-generated development token file. |
| `authorizedDomains` | list | `[]` | List of email domains allowed to authenticate. Empty allows all. |

### OAuth Section (`oauth`)
OAuth credentials for Web and CLI clients.

| Field | Description |
| :--- | :--- |
| `oauth.web.google.clientId` | Google OAuth client ID for web frontend. |
| `oauth.web.github.clientId` | GitHub OAuth client ID for web frontend. |
| `oauth.cli.google.clientId` | Google OAuth client ID for CLI loopback. |
| `oauth.cli.github.clientId` | GitHub OAuth client ID for CLI loopback. |

### Logging & Global
General service settings.

| Field | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `logLevel` | string | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`). |
| `logFormat` | string | `text` | Log output format (`text` or `json`). |

## Environment Variables
Server settings use a nested naming convention for environment variables.
- `SCION_SERVER_HUB_PORT` -> `hub.port`
- `SCION_SERVER_DATABASE_DRIVER` -> `database.driver`
- `SCION_SERVER_AUTH_DEVMODE` -> `auth.devMode`
- `SCION_SERVER_AUTH_AUTHORIZEDDOMAINS` -> `auth.authorizedDomains`
- `SCION_SERVER_LOG_LEVEL` -> `logLevel`

**Shorthand Environment Variables:**
- `SCION_AUTHORIZED_DOMAINS`: Maps to `auth.authorizedDomains` (comma-separated list).