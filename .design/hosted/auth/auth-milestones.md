# Authentication Implementation Milestones

This document tracks the phased implementation of Scion authentication.

*Last updated: 2026-01-31*

---

## Phase 0: Development Authentication (Interim)

- [x] Add `auth.devMode`, `auth.devToken`, `auth.devTokenFile` to config schema
- [x] Implement `InitDevAuth()` function
- [x] Add `--dev-auth` flag to `scion server start`
- [x] Implement `DevAuthMiddleware`
- [x] Add startup logging for dev token
- [ ] Add validation to block non-localhost + no-TLS + devMode
- [x] Add `WithDevToken()` option to `hubclient`
- [x] Add `WithAutoDevAuth()` option to `hubclient`
- [x] Add `SCION_DEV_TOKEN` environment variable support in CLI

---

## Phase 1: Web OAuth

- [x] OAuth provider integration (Google, GitHub)
- [x] Session cookie management
- [x] User creation/lookup on login
- [x] Hub auth endpoints (`/api/v1/auth/*`)

---

## Phase 2: CLI Authentication

- [x] `scion hub auth login` command
- [x] Localhost callback server (`pkg/hub/auth/localhost_server.go`)
- [ ] PKCE implementation
- [x] Credential storage (`pkg/credentials/store.go`)
- [x] `scion hub auth status` command
- [x] `scion hub auth logout` command

---

## Phase 2.5: Agent Authentication (sciontool)

*Added: 2026-01-31*

- [x] Hub-issued JWT tokens for agents (`pkg/hub/agenttoken.go`)
- [x] Agent token validation middleware
- [x] Token generation during agent provisioning
- [x] `SCION_HUB_TOKEN` environment variable in containers
- [x] sciontool hub client (`pkg/sciontool/hub/client.go`)
- [x] Agent status reporting to Hub
- [ ] Token refresh mechanism
- [ ] Scope-based authorization enforcement on endpoints

---

## Phase 3: API Keys

- [ ] API key generation endpoint
- [ ] API key validation middleware
- [ ] Key management UI in dashboard
- [ ] `scion hub auth set-key` command

---

## Phase 4: Security Hardening

- [ ] Rate limiting on auth endpoints
- [ ] Audit logging
- [ ] Token revocation lists
- [ ] Session invalidation on password change

---

## Phase 5: Unified Server Authentication

*Added: 2026-01-31*

- [ ] Implement unified `authMiddleware` with token type detection
- [ ] Implement `UserTokenService` for Hub-issued JWTs
- [ ] Add `/api/v1/auth/login` endpoint (OAuth token exchange for web)
- [ ] Update Koa proxy to use Hub-issued tokens
- [ ] Implement `Identity` interface for user/agent context
- [ ] Add trusted proxy support for Koa frontend
- [ ] Migrate from dev-only auth to unified middleware
- [ ] Implement token blacklist for revocation
- [ ] Add HTTPS enforcement in production

---

## Related Documents

- [Auth Overview](auth-overview.md) - Identity model and token types
- [Web Authentication](web-auth.md) - Browser-based OAuth flows
- [CLI Authentication](cli-auth.md) - Terminal-based authentication
- [Server Authentication](server-auth-design.md) - Hub server-side auth handling
- [Server Auth Setup](server-auth-setup.md) - API keys and dev authentication
- [Runtime Broker Auth](runtime-broker-auth.md) - Broker registration (future)
- [sciontool Auth](sciontool-auth.md) - Agent-to-Hub JWT authentication
