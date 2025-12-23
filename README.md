# Scion

A container-based orchestration tool for managing concurrent Gemini CLI agents.

## Overview

`scion` enables parallel execution of specialized Gemini CLI agents with isolated identities, credentials, and workspaces. It follows a Manager-Worker architecture where the host-side CLI orchestrates the lifecycle of isolated containers acting as independent agents.

## Key Features

- **Parallelism**: Run multiple agents concurrently as independent processes.
- **Isolation**: Strict separation of identities, credentials, and configuration.
- **Context Management**: Support for dedicated git worktrees when running within a git repository to prevent conflicts.
- **Specialization**: Role-based agent configuration via templates.
- **Interactivity**: Detached background operation with human-in-the-loop "attach" capability.

## Quick Start

### Initialize

```bash
scion grove init
```

### Start an Agent

```bash
scion start "Analyze this codebase" --name auditor --type security-auditor
```

### List Agents

```bash
scion list
```

### Attach to an Agent

```bash
scion attach auditor
```

## License

This project is licensed under the Apache License, Version 2.0. See the [LICENSE](LICENSE) file for details.
