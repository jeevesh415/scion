---
title: Team Workflow
description: Connecting to a Scion Hub for team collaboration.
---

While Scion works great in "Solo" mode for individual developers, its true power is realized in a "Hosted" architecture where teams can share state, infrastructure, and agent configurations.

## Connecting to a Hub

To join a team environment, you first need to connect your local CLI to the team's Scion Hub.

```bash
# Set the Hub endpoint
scion config set hub.endpoint https://scion.yourcompany.com

# Login via the browser
scion hub login
```

Once authenticated, your CLI will route agent operations through the Hub instead of running them purely locally.

## Registering a Grove

In a team environment, a "Grove" represents a shared project or repository.

1.  Navigate to your project directory.
2.  Register the grove: `scion hub register`
3.  The Hub will link your local project (via its Git remote) to a central Grove ID.

## Shared Infrastructure

When you start an agent in a team workflow, the Hub dispatches it to an available **Runtime Broker**.
- **Isolation**: Each team member's agents run in isolated containers.
- **Persistence**: Agent state and logs are centralized on the Hub, making them visible to other team members (based on permissions).
- **Resources**: Offload agent execution from your local machine to powerful remote servers or Kubernetes clusters.

## Managing Secrets

In a team workflow, secrets should be managed on the Hub rather than in local `.env` files.

```bash
# Set a secret for the current grove
scion hub secret set GITHUB_TOKEN=ghp_...

# The secret is securely stored in the Hub and injected into agents at runtime.
```

## Collaborating

- **Shared Visibility**: Use the Web Dashboard to see what agents your team is running.
- **Shared Templates**: Use centrally managed templates for consistent agent behavior across the team.
- **Attach to Remote Agents**: You can `scion attach` to an agent running on a remote Runtime Broker just as if it were local.
