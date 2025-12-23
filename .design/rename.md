# Project Rename: gswarm -> scion

This document tracks the comprehensive rename of the project from **gswarm** to **scion**.

## Overview
The goal is to replace all occurrences of `gswarm` with `scion` across the entire codebase, including filenames, directory names, Go module paths, imports, CLI commands, labels, and documentation.

## Plan

### 1. Preparation & Review
- [x] Conduct initial codebase search for `gswarm` (all cases).
- [x] Create this tracking document.

### 2. Infrastructure & Metadata
- [ ] Update `go.mod` module name.
- [ ] Rename hidden directory `.gswarm` to `.scion`.
- [ ] Update `.gitignore` to reflect the new directory name.
- [ ] Rename configuration file `gswarm.json` to `scion.json` (including templates).
- [ ] Rename `docs/gswarm-config-reference.md` to `docs/scion-config-reference.md`.

### 3. Codebase Content (Automated Replacement)
- [ ] Replace `github.com/ptone/gswarm` with `github.com/ptone/scion` in all Go files.
- [ ] Replace `gswarm` with `scion` in all strings, comments, and documentation.
- [ ] Replace `GSWARM` with `SCION` (e.g., environment variables).
- [ ] Replace `Gswarm` with `Scion` (e.g., struct names, titles).

### 4. CLI & Runtime
- [ ] Update CLI `Use` and `Short`/`Long` descriptions in `cmd/`.
- [ ] Update Docker labels:
    - `gswarm.agent` -> `scion.agent`
    - `gswarm.name` -> `scion.name`
    - `gswarm.tmux` -> `scion.tmux`
- [ ] Update Tmux session name from `gswarm` to `scion`.
- [ ] Update default paths in `pkg/config/paths.go` and `pkg/config/init.go`.

### 5. Scripts & Tooling
- [ ] Update `hack/*.sh` scripts.
- [ ] Update `cli-tmux.Dockerfile`.

### 6. Verification
- [ ] Run `go mod tidy`.
- [ ] Run `go build ./...`.
- [ ] Run existing tests (`go test ./...`).
- [ ] Verify CLI functionality (`scion grove init`, `scion start`, etc.).

---

## Tracking Log

- **2025-12-23**: Initialized rename plan.
- **2025-12-23**: Updated go.mod, renamed .gswarm to .scion, updated imports and file contents.
- **2025-12-23**: Renamed Gemini Swarm to Scion.
- **2025-12-23**: Renamed swarm.md to scion.md and updated internal terminology.
- **2025-12-23**: Cleaned up redundant "Scion (scion)" mentions.
- **2025-12-23**: Verified build and tests pass.
- **2025-12-23**: Replaced "swarm" with "grove" in documentation, design docs, and user-facing CLI messages.
