# scion Implementation Milestones

This document breaks down the implementation of `scion` into independent stages, allowing for iterative development and verification.

## Milestone 1: Project Scaffolding & Configuration
**Goal**: Establish the basic CLI structure and filesystem management.

- [x] Implement `scion grove init` (**Completed**)
    - [x] Create `.scion/` directory structure in the current repo.
    - [x] Seed `.scion/templates/default` with basic agent structure.
    - [x] Create global `~/.scion/` structure for Playground groves.
- [x] Implement Template Loading (**Completed**)
    - [x] Logic to find and load templates (Project-local vs. Global).
    - [x] Simple inheritance (custom template merged with `default`).

## Milestone 2: Container Runtime Abstraction
**Goal**: Create a unified interface for managing containers across different platforms.

- [x] Implement `Runtime` interface (Go package) (**Completed**)
    - [x] Methods: `RunDetached`, `Stop`, `List`, `GetLogs`.
- [x] Implement macOS `container` backend (**Completed**)
    - [x] Integrate configuration loading (`GEMINI_SANDBOX` env, `settings.json`). (**Completed**)
    - [x] Implement Network Management (**N/A** - checked `container` CLI and it has no network subcommands).
- [x] Implement Linux `docker` backend (**Completed**)
- [x] Verify basic container launch with TTY allocation (**Completed**)

## Milestone 3: Basic Agent Provisioning
**Goal**: Launch isolated agents without Git Worktree complexity.

- [ ] Implement `scion start` (v1) (**In Progress**)
    - [x] Select template.
    - [x] Copy template to `.scion/agents/<name>/home`.
    - [x] Implement Environment & Credential Propagation (API keys, gcloud config). (**Completed**)
    - [x] Launch container with home directory mounted to `/home/gemini`.
- [x] Implement basic ID management to prevent name collisions (**Completed**)
- [ ] Verify agent has unique identity and persistent history (**Pending**)

## Milestone 4: Git Worktree Integration
**Goal**: Enable concurrent agents to work on the same repository safely (when running within a git repo).

- [ ] Implement Worktree Manager (**Pending**)
    - [ ] Logic to detect if current project is a git repo.
    - [ ] Logic to create worktrees in `../.scion_worktrees/` if in a git repo.
    - [ ] Automatic branch creation for the agent.
- [ ] Update `scion start` (v2) (**Pending**)
    - [ ] Conditionally mount worktree to `/workspace` in the container.
    - [ ] Implement macOS-specific path isolation checks.
- [ ] Verify two agents can run in the same grove with different file states (**Pending**)

## Milestone 5: Grove Management & Observability
**Goal**: Provide visibility into running agents and manage their lifecycle.

- [ ] Implement `scion list` (**Pending**)
    - [ ] Query container runtime for running agents.
    - [ ] Parse and display agent status from `.gemini-status.json`.
- [ ] Implement `scion stop` (**Pending**)
    - [ ] Graceful container termination.
    - [ ] Git worktree cleanup.
- [ ] Implement Playground Grove support (global context) (**Pending**)

## Milestone 6: Interactivity & Human-in-the-Loop
**Goal**: Support "detached" operation with the ability to intervene.

- [ ] Implement `scion attach` (**Pending**)
    - [ ] Connect host TTY to the running container's session.
    - [ ] Ensure escape sequences (Ctrl-P, Ctrl-Q) work for detaching.
- [ ] Implement status-driven alerts. (**Pending**)
- [ ] Support "Yolo" mode flag in `start` (**Pending**)

## Milestone 7: Advanced Template Management
**Goal**: Facilitate easy customization of agent personas.

- [ ] Implement `scion templates` subcommands (**Pending**)

    - [ ] `list`, `create`, `delete`.

- [ ] Implement Extension management (**Pending**)

    - [ ] `extensions install` (modifies `settings.json` in the template).



## Current Issues & Debugging Tasks

- [ ] **Issue**: [Auth Dialog Appears Despite Valid Credentials](./issues/auth-dialog.md)

- [ ] **Issue**: [Apple Native Container Does Not Support Attach](./issues/apple-container-attach.md)
