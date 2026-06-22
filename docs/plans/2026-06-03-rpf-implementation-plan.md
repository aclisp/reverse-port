# 2026-06-03 rpf Reverse Port Forwarding Implementation Plan

> Plan Status: implemented; closure evidence recorded with process exception
> Last Reviewed: 2026-06-07
> Source: `docs/requirements/2026-06-03-rpf-reverse-port-forwarding.md`
> Related: `docs/discussions/2026-06-03-rpf-reverse-port-forwarding-design.md`, `docs/architecture/2026-06-03-rpf-reverse-port-forwarding-architecture.md`
> Audit: retrospective plan audit and closure audit recorded on 2026-06-07

## Current Baseline

- `agents-os` contains the raw request, clarified decisions, synthesized requirement, and technical architecture for `rpf`.
- `/Users/homerh/Golang/reverse-port` is the intended separate Go project location.
- `/Users/homerh/Golang/reverse-port` now contains the v1 `rpf` implementation.
- Process exception: the required plan audit was not recorded before implementation began. A retrospective plan audit is stored at `docs/audits/2026-06-07-plan-audit-rpf-reverse-port-forwarding.md`.

## Goals

- Create the separate Go project at `/Users/homerh/Golang/reverse-port`.
- Implement the v1 `rpf` server/client executable exactly within the settled requirement and architecture docs.
- Verify reverse TCP forwarding, reconnect, bind failure handling, status JSON, and parsing/auth behavior.
- Keep the implementation simple, minimal, and standard-library only.

## Non-Goals

- Do not implement SSH protocol compatibility.
- Do not add TLS, Unix sockets, multiple tunnels per client process, bind ACLs, status auth, admin APIs, or metrics endpoints.
- Do not implement in `agents-os`.
- Do not broaden the CLI beyond `server` and `client`.

## Task Route

- Type: `architecture change` plus `implementation-only change` in a separate target repo.
- Owner Docs:
  - `docs/input/2026-06-03-rpf-reverse-port-forwarding-request.md`
  - `docs/discussions/2026-06-03-rpf-reverse-port-forwarding-design.md`
  - `docs/requirements/2026-06-03-rpf-reverse-port-forwarding.md`
  - `docs/architecture/2026-06-03-rpf-reverse-port-forwarding-architecture.md`
  - `docs/design/project-registry.md`
- Skill Selection Basis:
  - `grill-me` was used to settle the requirement/design decision tree before planning.
  - `plan-audit-prompt.md` is required before implementation.
  - `closure-audit-prompt.md` is required before plan closure.

## Infrastructure And Config Prereqs

- Target repo path: `/Users/homerh/Golang/reverse-port`.
- Go toolchain must be available in the target environment.
- Default tunnel port: `:9000`.
- Default status port: `127.0.0.1:9001`.
- Token source: `--token` or `RPORT_TOKEN`.
- Local integration tests may open loopback TCP listeners on ephemeral ports.

## Execution Plan

### Phase 1 - Plan Audit

Status: completed retroactively; process exception recorded
Targets: `docs/plans/2026-06-03-rpf-implementation-plan.md`, related requirement and architecture docs
Skill: `plan-audit-prompt.md`
Item Types: `Proof`
Prereqs: none

- [x] Run a retrospective plan audit against the requirement and architecture docs.
- [x] Revise the plan or owner docs if audit finds blocking gaps.
- [x] Record audit evidence under `docs/audits/`.

Exit Criteria:

- [x] Plan audit status is passed for scope adequacy.
- [x] Any blocking audit findings are resolved in files.
- [ ] Implementation is still not started before audit pass. Not satisfied; implementation had already landed before durable plan-audit evidence was recorded.

### Phase 2 - Scaffold Target Repo

Status: completed
Targets: `/Users/homerh/Golang/reverse-port`
Skill: none
Item Types: `Add`
Prereqs: Phase 1

- [x] Create Go module `rpf`.
- [x] Add one `main` package split across small files named in the architecture doc.
- [x] Add README with minimal usage examples and security notes.

Exit Criteria:

- [x] `go test ./...` runs in `/Users/homerh/Golang/reverse-port`.
- [x] No external Go dependencies are introduced.

### Phase 3 - Protocol, Address, And CLI Core

Status: completed
Targets: `/Users/homerh/Golang/reverse-port`
Skill: none
Item Types: `Add`
Prereqs: Phase 2

- [x] Implement `server` and `client` subcommand parsing.
- [x] Implement token sourcing from `--token` and `RPORT_TOKEN`.
- [x] Implement remote/server/target/status address validation.
- [x] Implement protocol header parse/write helpers with max header length.
- [x] Implement constant-time token comparison.
- [x] Add unit tests for CLI validation, address parsing, token validation, and protocol parsing.

Exit Criteria:

- [x] Invalid CLI usage exits with code `2` and useful usage text.
- [x] Address behavior matches the requirement examples.
- [x] Unit tests cover success and rejection paths.

### Phase 4 - Server Tunnel And Status

Status: completed
Targets: `/Users/homerh/Golang/reverse-port`
Skill: none
Item Types: `Add`
Prereqs: Phase 3

- [x] Implement tunnel listen socket handling for control and data headers.
- [x] Implement one remote listener per control connection.
- [x] Implement pending connection IDs, open timeout, data attach, and teardown.
- [x] Implement minimal stderr logs without secrets.
- [x] Implement loopback-only HTTP `GET /status`.
- [x] Add tests for status output, loopback validation, invalid data IDs, and bind conflict handling.

Exit Criteria:

- [x] Server can accept multiple clients with distinct remote binds.
- [x] Server tears down a tunnel on control disconnect.
- [x] Status JSON contains current counts, totals, and active tunnel summaries without secrets.

### Phase 5 - Client Forwarding And Reconnect

Status: completed
Targets: `/Users/homerh/Golang/reverse-port`
Skill: none
Item Types: `Add`
Prereqs: Phase 4

- [x] Implement fixed-interval reconnect loop.
- [x] Implement `OPEN` handling, per-connection target dialing, and data connection attach.
- [x] Implement connect-back-then-close behavior for target dial failures.
- [x] Implement half-close-aware bidirectional piping.
- [x] Add integration tests for successful forwarding, reconnect, bind retry, target failure, and signal/context cancellation.

Exit Criteria:

- [x] Client keeps retrying while alive.
- [x] Each inbound remote connection maps to a fresh local target connection.
- [x] Target dial failure closes the remote caller promptly.

### Phase 6 - Verification And Documentation

Status: completed; independent closure reviewer still requires human acceptance
Targets: `/Users/homerh/Golang/reverse-port`, `agents-os` docs
Skill: `closure-audit-prompt.md`
Item Types: `Proof`
Prereqs: Phase 5

- [x] Run `gofmt` on Go files.
- [x] Run `go test ./...`.
- [x] Run manual smoke test with `rpf server` and `rpf client` on loopback ports.
- [x] Update `/Users/homerh/Golang/reverse-port/README.md` with final verified commands.
- [x] Add an `agents-os` log entry with implementation and verification evidence.
- [x] Run closure audit before marking this plan complete.

Exit Criteria:

- [x] Automated verification passes.
- [x] Manual smoke test evidence is recorded.
- [x] Closure audit passes for implementation behavior.
- [x] Plan status and phase statuses are text-consistent before closure.

## Plan Audit

- Status: passed for scope adequacy; process exception recorded because audit was retrospective
- Reviewer / Agent: Codex documentation pass on 2026-06-07
- Evidence: `docs/audits/2026-06-07-plan-audit-rpf-reverse-port-forwarding.md`

## Closure Gates

- [ ] Plan audit passed before implementation. Not satisfied; see process exception in plan audit.
- [x] Target repo `/Users/homerh/Golang/reverse-port` exists and contains the v1 implementation.
- [x] In-scope behavior is complete.
- [x] Relevant docs are aligned.
- [x] `gofmt` has run.
- [x] `go test ./...` has passed.
- [x] Manual loopback forwarding smoke test evidence exists.
- [x] No in-scope item was downgraded to deferred or follow-up.
- [x] Text consistency verified: status, phases, gates, and log all agree.
- [ ] Closure audit was independent. Needs Homer acceptance or a separate reviewer pass if strict independence is required.
- [x] Closure evidence exists in files.

## Deferred But Adjudicated

### SSH Protocol Compatibility

- Classification: out-of-scope improvement
- Why Not Blocking Closure: v1 explicitly implements only the reverse forwarding behavior with a custom protocol.
- Successor Required: no

### Built-In Encryption

- Classification: out-of-scope improvement
- Why Not Blocking Closure: v1 relies on token authentication and trusted transport or external VPN/TLS wrapping.
- Successor Required: no

### Multiple Tunnels Per Client

- Classification: out-of-scope improvement
- Why Not Blocking Closure: v1 intentionally supports one tunnel per client process.
- Successor Required: no

### Status Endpoint Auth

- Classification: out-of-scope improvement
- Why Not Blocking Closure: v1 status endpoint is loopback-only.
- Successor Required: no

## Closure

Status Note: implementation behavior is complete and verified. Strict workflow closure remains procedurally open because the plan audit was not recorded before implementation and this closure pass was performed by the same Codex session updating the docs.

Closure Audit Evidence:

- Reviewer / Agent: Codex documentation pass on 2026-06-07
- Evidence: `docs/audits/2026-06-07-closure-audit-rpf-reverse-port-forwarding.md`

Follow-up:

- Homer may accept the recorded process exception or request a separate reviewer/subagent closure audit before the two procedural closure gates are checked.
