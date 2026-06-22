# rpf Reverse Port Forwarding Architecture

## Scope

This document owns the v1 technical architecture for the future `rpf` Go project at `/Users/homerh/Golang/reverse-port`.

Related requirement: `docs/requirements/2026-06-03-rpf-reverse-port-forwarding.md`

## Runtime Shape

- One executable: `rpf`.
- One Go module: `rpf`.
- One Go package: `main`.
- Small file split:
  - `main.go`: subcommand dispatch and usage.
  - `server.go`: tunnel server, remote listener lifecycle, status state.
  - `client.go`: reconnect loop, control handling, target dialing.
  - `protocol.go`: header parsing, writing, auth comparison helpers.
  - `addr.go`: CLI address parsing and validation.
  - `pipe.go`: half-close-aware bidirectional copy.

The implementation should remain standard-library only.

## Connection Model

The server has one tunnel listen socket. Both control and data connections connect to the same address.

Each client process owns:

- one persistent control connection
- one requested remote listener on the server
- many short-lived data connections, one per inbound remote TCP connection

Each server remote listener belongs to exactly one authenticated control connection. When that control connection ends, the server closes:

- the remote listener
- pending remote inbound sockets
- active data pipes for that tunnel

## Protocol

The protocol is line-oriented for initial headers. After a valid data header, both sides switch to raw byte piping.

### Control Header

```text
CONTROL secret 127.0.0.1:8080 127.0.0.1:3000\n
```

Server responses:

```text
OK\n
ERR remote bind failed\n
```

After `OK`, the server sends open requests on the control connection:

```text
OPEN 9f4c0a8c5a3e2a73d913c35d9c8712b0\n
```

### Data Header

```text
DATA secret 9f4c0a8c5a3e2a73d913c35d9c8712b0\n
```

If valid, the server sends no response and starts raw piping immediately. If invalid, the server may write `ERR\n` and closes.

### Header Safety

- Header reads must enforce a maximum length.
- Tokens must be non-empty and contain no whitespace.
- Addresses must contain no whitespace.
- Token comparison must use constant-time comparison.
- Data attach errors should not reveal whether a connection ID exists.

## Connection IDs

- Server-generated using `crypto/rand`.
- Encoded as hex.
- Single-use.
- Stored in a pending map until attached or expired.
- Removed immediately when a valid data socket attaches.
- Expired after `--open-timeout`.

## Address Parsing

Remote bind parsing must preserve OpenSSH-like semantics:

| User input | Server bind |
|------------|-------------|
| `8080` | `127.0.0.1:8080` |
| `127.0.0.1:8080` | `127.0.0.1:8080` |
| `:8080` | `:8080` |
| `*:8080` | `:8080` |
| `0.0.0.0:8080` | `0.0.0.0:8080` |
| `[::1]:8080` | `[::1]:8080` |
| `[::]:8080` | `[::]:8080` |

Remote and target port `0` are invalid.

`--server`, `--remote`, and `--target` may accept hostnames. Hostname resolution happens on the machine using the address:

- server resolves/binds `--remote`
- client resolves/dials `--server` and `--target`

## Piping

Active forwarded connections use two copy directions:

```text
remote -> data -> target
target -> data -> remote
```

When a copy direction reaches EOF, call `CloseWrite` on the opposite TCP connection when available. Close both sockets when both directions end or cancellation happens.

No idle data deadlines are used in v1.

## Client Reconnect

The client reconnect loop runs until context cancellation.

- Fixed interval default: `5s`.
- Reconnects after control disconnect.
- Retries after remote bind failure.
- Invalid local configuration exits immediately before the loop.

## Status HTTP Server

The server starts a separate loopback-only HTTP listener.

- Default: `127.0.0.1:9001`.
- Endpoint: `GET /status`.
- Auth: none in v1 because only loopback binding is supported.
- Non-loopback status listen addresses are rejected in v1.
- Startup fails if status listener cannot bind.

Status tracks:

- current active tunnel count
- current pending connection count
- current active connection count
- cumulative accepted/rejected control connections
- cumulative accepted/rejected data connections
- cumulative remote inbound connections
- active tunnel summaries with remote, target, client address, pending count, and active count

Status must never include tokens or protocol secrets.

## Logging

Logs go to stderr. Keep them operational and minimal:

- server startup and status startup
- remote listener ready/closed
- client connect/disconnect
- bind failures
- target dial failures
- malformed versus unauthorized control requests
- minimal data attach rejection logs

Do not log token values.

## Verification Strategy

Expected verification in `/Users/homerh/Golang/reverse-port` after implementation:

- `gofmt` on all Go files.
- `go test ./...`.
- Unit tests for address parsing, token validation, protocol parsing, and status loopback validation.
- Integration tests using local TCP listeners for:
  - successful reverse forwarding
  - bind conflict retry behavior
  - client reconnect after server restart or control close
  - target dial failure closes remote caller
  - status JSON counts and tunnel summary
