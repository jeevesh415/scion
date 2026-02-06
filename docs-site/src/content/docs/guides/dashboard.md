---
title: Web Dashboard
description: Using the Scion Web Dashboard for visualization and control.
---

The Scion Web Dashboard provides a visual interface for managing your agents, groves, and runtime brokers. It complements the CLI by providing real-time status updates and easier management of complex environments.

## Overview

The dashboard is organized into several key areas:

### Dashboard Home
The landing page provides an overview of your active agents across all groves and the status of your runtime brokers.

### Groves
View and manage your registered groves.
- **Register Grove**: Connect a new repository to the Hub.
- **Grove Settings**: Manage environment variables and secrets for the entire grove.
- **Agent List**: See all agents belonging to the grove.

### Agents
Detailed view for individual agents.
- **Status Monitoring**: Real-time view of agent lifecycle (Starting, Thinking, Waiting, etc.).
- **Logs**: Streamed logs from the agent container.
- **Terminal (Upcoming)**: Interactive terminal access to the agent's workspace.
- **Lifecycle Control**: Start, stop, restart, or delete agents from the UI.

### Runtime Brokers
Monitor the infrastructure nodes where your agents are executing.
- **Status**: See which brokers are online and their current load.
- **Configuration**: View broker capabilities (Docker, K8s, etc.).

## Authentication

The dashboard supports several authentication methods:
- **OAuth (Google/GitHub)**: For standard user access.
- **Development Auto-login**: For local development.

See the [Authentication Guide](/guides/auth) for setup instructions.

## API Proxying
The dashboard BFF (Backend-for-Frontend) acts as a secure proxy for the Hub API, handling token injection and session management so the browser never handles raw API keys or long-lived tokens directly.
