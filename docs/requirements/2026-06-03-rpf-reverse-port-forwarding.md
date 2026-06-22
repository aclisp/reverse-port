# Feature: rpf Reverse TCP Port Forwarding

## Goal

Build a small Go executable named `rpf` that provides the reverse TCP forwarding behavior of:

```text
ssh -R [bind_address:]port:host:hostport
```

The tool must let a remote server listen on a requested TCP address and forward each inbound connection over a client-maintained tunnel to a local target dialed by the client.

## In Scope

- One Go executable named `rpf`.
- Two subcommands: `server` and `client`.
- Custom minimal protocol, not SSH compatibility.
- One reverse forward per client process.
- One reusable server that accepts multiple simultaneous clients.
- Token authentication using `--token` or `RPORT_TOKEN`.
- Client-sent remote bind address and target address.
- TCP-only forwarding.
- IPv4, IPv6 bracket syntax, and hostnames where Go TCP address syntax supports them.
- Client reconnect forever at a fixed configurable interval.
- Server HTTP status endpoint on loopback only.
- Standard-library-only implementation.

## Out Of Scope

- SSH protocol compatibility.
- Built-in TLS or encryption.
- Unix socket forwarding.
- Multiple tunnels in one client process.
- Bind ACLs beyond token authentication and OS permissions.
- Status endpoint authentication.
- Non-loopback status binding.
- Remote or target port `0`.
- Combined OpenSSH-style `remote:target` CLI syntax.
- Protocol-prefixed address syntax such as `tcp://host:port`.
- Automatic client identity, client accounts, per-client tokens, metrics endpoint, or admin API.

## Main User Flows

### Start Server

1. Operator starts `rpf server`.
2. Server reads token from `--token` or `RPORT_TOKEN`.
3. Server listens for tunnel control/data connections, default `:9000`.
4. Server listens for HTTP status, default `127.0.0.1:9001`.
5. Server rejects startup if token is missing, tunnel listen fails, or status listen fails.

### Start Client Tunnel

1. Operator starts `rpf client --server server.example:9000 --remote 127.0.0.1:8080 --target 127.0.0.1:3000`.
2. Client validates configuration.
3. Client connects to server and sends authenticated control request with remote and target.
4. Server validates token and remote address, then attempts to bind remote listener.
5. If bind succeeds, server replies `OK`; client enters forwarding loop.
6. If bind fails, client logs the error, sleeps for `--reconnect-interval`, and tries again until stopped.

### Forward Inbound Connection

1. A remote caller connects to the server-side remote listener.
2. Server generates a random connection ID and sends `OPEN 9f4c0a8c5a3e2a73d913c35d9c8712b0` on the client's control connection.
3. Client dials its local `--target`.
4. Client opens a data connection to the same server listen address and sends `DATA secret 9f4c0a8c5a3e2a73d913c35d9c8712b0`.
5. If target dial succeeded, client pipes target socket to data socket.
6. Server attaches the data socket to the pending remote caller and pipes bytes both ways.
7. If target dial failed, client still opens and closes the data connection so the server closes the remote caller promptly.

### Reconnect

1. If the control connection drops, server closes that client's remote listener, pending opens, and active pipes.
2. If the client process is still alive, it sleeps for the fixed reconnect interval and reconnects.
3. Client re-authenticates and requests the same remote bind.

### Check Status

1. Operator requests `GET /status` from the loopback status listener.
2. Server returns JSON with status, current counts, cumulative totals, and active tunnel summaries.
3. Response never includes tokens or protocol secrets.

## CLI Contract

### Server

```bash
rpf server [--listen :9000] [--status-listen 127.0.0.1:9001] [--open-timeout 10s] [--token secret]
```

- `--listen` defaults to `:9000`.
- `--status-listen` defaults to `127.0.0.1:9001`.
- `--status-listen` must be loopback in v1.
- `--open-timeout` defaults to `10s`.
- `--token` overrides `RPORT_TOKEN`.
- A token is required.

### Client

```bash
rpf client --server host:port --remote [bind_address:]port --target host:hostport [flags]
```

- `--server` is required.
- `--remote` is required.
- `--target` is required.
- `--token` overrides `RPORT_TOKEN`.
- A token is required.
- `--reconnect-interval` defaults to `5s`.

Invalid CLI usage prints subcommand usage and exits with code `2`.

## Address Rules

- `--remote 8080` means `127.0.0.1:8080`.
- `--remote 127.0.0.1:8080` binds loopback explicitly.
- `--remote :8080` means all interfaces.
- `--remote '*:8080'` means all interfaces.
- `--remote 0.0.0.0:8080` binds all IPv4 interfaces.
- `--remote '[::1]:8080'` uses standard bracketed IPv6 syntax.
- `--remote '[::]:8080'` binds all IPv6 interfaces.
- `--server`, `--remote`, and `--target` may use hostnames where applicable.
- Empty or whitespace-containing tokens are invalid.
- Whitespace-containing addresses are invalid.
- Remote and target port `0` are invalid.

## Server Status Contract

`GET /status` returns JSON similar to:

```json
{
  "status": "ok",
  "serverListen": ":9000",
  "statusListen": "127.0.0.1:9001",
  "current": {
    "activeTunnels": 1,
    "pendingConnections": 0,
    "activeConnections": 2
  },
  "totals": {
    "acceptedControlConnections": 4,
    "rejectedControlConnections": 1,
    "acceptedDataConnections": 20,
    "rejectedDataConnections": 2,
    "remoteConnections": 18
  },
  "tunnels": [
    {
      "remote": "127.0.0.1:8080",
      "target": "127.0.0.1:3000",
      "client": "203.0.113.5:52344",
      "activeConnections": 2,
      "pendingConnections": 0
    }
  ]
}
```

## Roles / Permissions

- No application roles.
- Token possession authorizes a client to request any bind address/port that the server process and OS allow.
- The token must not appear in logs, status responses, or error messages.

## Edge Cases

- Remote bind address already in use: client logs and retries forever.
- Remote bind permission denied: treated as normal bind failure and retried forever.
- Server control connection drops: server tears down the owning tunnel; client reconnects if still alive.
- Unknown, expired, or reused data connection ID: server rejects and closes immediately.
- Client target unavailable: client opens the matching data connection and closes it immediately.
- Active forwarded data streams have no idle deadlines.
- Pending data attach waits no longer than `--open-timeout`.
- SIGINT/SIGTERM stops server or client cleanly.

## Open Questions

- No known blocking requirement questions remain.
- Implementation was completed in `/Users/homerh/Golang/reverse-port`. The required pre-implementation plan audit was missed and is recorded as a process exception in `docs/audits/2026-06-07-plan-audit-rpf-reverse-port-forwarding.md`.

## Acceptance Criteria

- [x] `rpf server` starts with `--token` or `RPORT_TOKEN`, listens on `:9000`, and exposes loopback `GET /status`.
- [x] `rpf client` connects to the server, requests one remote listener, and forwards inbound remote TCP connections to the local target.
- [x] Multiple clients can attach to one server when remote binds do not conflict.
- [x] Bind conflicts and bind permission errors retry forever on the client.
- [x] Client reconnects forever at fixed interval while the process remains alive.
- [x] Control disconnect tears down that client's server-side listener and active pipes.
- [x] Address parsing follows the confirmed OpenSSH-like remote bind semantics.
- [x] Token validation rejects missing, empty, whitespace-containing, or incorrect tokens.
- [x] Data connection IDs are random, single-use, and rejected when invalid.
- [x] Status JSON includes current counts, totals, active tunnel summaries, and target addresses, but no secrets.
- [x] Go implementation uses only the standard library.
- [x] Verification covers unit tests for parsing/protocol helpers and integration tests for end-to-end forwarding, reconnect, bind failure, and status output.
