---
title: Setting up the Scion Hub
description: Installation and configuration of the Scion Hub (State Server).
---

The **Scion Hub** is the central brain of a hosted Scion architecture. It maintains the state of all agents, groves, and runtime brokers, and provides the API used by the CLI and Web Dashboard.

## Running the Hub

The Hub is part of the main `scion` binary. You can start it using the `server` command:

```bash
# Start the Hub and a local Runtime Broker
scion server

# Start ONLY the Hub
scion server --hub
```

## Configuration

The Hub looks for a configuration file at `~/.scion/server.yaml`.

### Basic Example
```yaml
hub:
  port: 9810
  host: 0.0.0.0
database:
  driver: sqlite
  url: hub.db
auth:
  devMode: true
logLevel: info
```

See the [Server Configuration Reference](/reference/server-config) for all available fields.

## Persistence

The Hub requires a database to store its state.

### SQLite (Default)
Ideal for local development or single-node deployments. The database is a single file.
```yaml
database:
  driver: sqlite
  url: /path/to/your/hub.db
```

### PostgreSQL (Production)
Recommended for high-availability or multi-node deployments.
```yaml
database:
  driver: postgres
  url: "postgres://user:password@localhost:5432/scion?sslmode=disable"
```

## Storage Backends

The Hub stores agent templates and other artifacts.

- **Local File System**: Default. Stores files in `~/.scion/storage`.
- **Google Cloud Storage (GCS)**: Recommended for cloud deployments. Set the `--storage-bucket` flag or use `SCION_STORAGE_BUCKET`.

## Deployment

### Docker
The Hub is available as a Docker image.

```bash
docker run -p 9810:9810 \
  -v ~/.scion:/root/.scion \
  ghcr.io/ptone/scion-hub:latest
```

### Kubernetes
A Helm chart is available in the repository under `deploy/charts/scion-hub`.

### Cloud Run (GCP)
The Hub is stateless (except for the database) and can be deployed to Google Cloud Run. Use a managed Cloud SQL instance for the database and a GCS bucket for storage.

## Monitoring

The Hub exposes health check endpoints:
- `/healthz`: Basic liveness check.
- `/readyz`: Readiness check (verifies database connectivity).

Logs are output to `stdout` in either `text` (default) or `json` format, suitable for collection by systems like Fluentd, Cloud Logging, or Prometheus.
