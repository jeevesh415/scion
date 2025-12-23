# Developer QA Walkthrough: scion

This document provides step-by-step instructions for manually building and verifying the core functionality of `scion`. 

**Recommendation**: To avoid "dogfooding collisions" (where `scion` clobbers its own source templates during testing), always perform manual QA in an isolated peer directory.

## Prerequisites

- Go 1.21+
- A container runtime:
    - **macOS**: `container` (Apple Virtualization Framework CLI) or Docker/Podman.
    - **Linux**: Docker or Podman.
- (Optional) `gemini` CLI installed and authenticated.

---

## 1. Setup Test Environment

### Create an isolated peer directory
```bash
# Assuming you are in the scion source root
mkdir -p ../scion-test
cd ../scion-test
# Optional: git init  (scion works in any directory, but supports git repositories)
```

### Build the binary to the test location
```bash
# From the scion-test directory, build the source from the peer directory
go build -o ../scion-test/scion ./main.go
```

### Initialize the test project
```bash
./scion grove init
```
**Verification**:
- Check for `.scion/` in the `scion-test` directory.
- Check for `~/.scion/` (global config) in your home directory.
- Verify `.scion/templates/default/.gemini/settings.json` exists in the test project.

---

## 2. Verify Authentication Discovery

`scion` should pick up keys from your environment or your host's `settings.json`.

### Case A: Environment Variable
```bash
export GEMINI_API_KEY="test-key-123"
# Run start
./scion start "test auth" --name qa-auth-env
```
**Verification**:
- Run `docker inspect qa-auth-env` (or `container inspect`).
- Confirm `GEMINI_API_KEY=test-key-123` is in the `Env` list.

### Case B: Settings JSON Fallback
1. Unset the env var: `unset GEMINI_API_KEY`
2. Ensure `~/.gemini/settings.json` has an `"apiKey": "config-key-456"` field.
3. Run: `./scion start "test settings" --name qa-auth-config`

**Verification**:
- Inspect the container.
- Confirm `GEMINI_API_KEY=config-key-456` is present.

---

## 3. Verify ADC (Service Account) Propagation

### Setup
```bash
export GOOGLE_APPLICATION_CREDENTIALS="/tmp/test-creds.json"
echo '{"type": "service_account"}' > /tmp/test-creds.json
./scion start "test adc" --name qa-adc
```

**Verification**:
- Inspect the container mounts (Binds).
- Confirm `/tmp/test-creds.json` is mounted to `/home/gemini/.config/gcp/application_default_credentials.json` as `ro`.
- Confirm the environment variable `GOOGLE_APPLICATION_CREDENTIALS` inside the container points to the internal path.

---

## 4. Verify Runtime Selection

### Force Docker
```bash
GEMINI_SANDBOX=docker ./scion start "force docker" --name qa-runtime-docker
```
**Verification**:
- Verify the container was created in Docker (`docker ps`).

### Force Apple Container (macOS only)
```bash
GEMINI_SANDBOX=container ./scion start "force apple" --name qa-runtime-apple
```
**Verification**:
- Verify the container was created in Apple `container` (`container list`).

### Verify --no-auth
```bash
export GEMINI_API_KEY="should-not-appear"
./scion start "test no-auth" --name qa-no-auth --no-auth
```
**Verification**:
- Inspect the container.
- Confirm `GEMINI_API_KEY` is **NOT** present in the environment variables.

---

## 5. Cleanup

After testing, remove the agents and the test directory:
```bash
# Docker
docker rm -f $(docker ps -a -q --filter "label=scion.agent=true")

# Apple Container
container stop $(container list -a --format json | jq -r '.[] | select(.configuration.labels["scion.agent"]=="true") | .id')
container rm $(container list -a --format json | jq -r '.[] | select(.configuration.labels["scion.agent"]=="true") | .id')

# Filesystem
cd ..
rm -rf scion-test
```