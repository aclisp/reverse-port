# rpf

[README](README.md) | [中文文档](README_zh.md)

`rpf` is a small reverse TCP port forwarding tool with behavior similar to:

```text
ssh -R [bind_address:]port:host:hostport
```

It uses a custom minimal protocol, not SSH. The v1 implementation is TCP-only,
standard-library only, and supports one reverse forward per client process.

## Features

- **Reverse TCP forwarding** similar to `ssh -R`, with the server binding a
  requested remote address and forwarding each inbound connection to a
  client-side target.
- **Small single-binary deployment** with only two subcommands: `server` and
  `client`.
- **Standard-library-only Go implementation** with no external runtime services,
  databases, or package dependencies.
- **Reusable multi-client server** where each authenticated client owns one
  remote listener, and multiple clients can share the same server when their
  requested remote binds do not conflict.
- **Client-selected forwarding contract**: the client sends both `--remote` and
  `--target`, so the server stays generic and does not need per-tunnel config
  files.
- **OpenSSH-like remote bind semantics** for common forms such as `8080`,
  `127.0.0.1:8080`, `:8080`, `*:8080`, and bracketed IPv6 addresses.
- **Separate control and data connections on one server port**: the first
  protocol line identifies `CONTROL` or `DATA`, then valid data connections
  switch to raw TCP piping.
- **Fresh target connection per inbound caller**, so every remote TCP connection
  maps to a new client-side target connection.
- **Reconnect-oriented client behavior**: bind failures, permission errors, and
  control disconnects are retried at a fixed configurable interval while the
  client process stays alive.
- **Stale tunnel cleanup** through server-initiated `PING` / `PONG` heartbeats,
  plus deterministic SIGINT/SIGTERM cleanup for server and client processes.
- **Half-close-aware bidirectional piping** for normal TCP EOF behavior, with no
  idle deadline on active forwarded streams.
- **Loopback-only status endpoint** at `GET /status`, reporting current counts,
  cumulative totals, and active tunnel summaries without tokens or connection
  IDs.
- **Server-side resource controls** for pending opens, active forwarded
  connections, and pending data attach timeout, plus a fixed 10s initial header
  read timeout.
- **Token authentication without secret logging**: tokens come from `--token` or
  `RPORT_TOKEN`, are compared in constant time, and are excluded from status and
  logs.
- **Explicit security boundary**: token auth is not encryption. Use a trusted
  network, VPN, TLS wrapper, or another transport security layer when traffic
  crosses untrusted networks.
- **Intentional v1 scope limits**: TCP only, custom protocol only, one tunnel per
  client process, no SSH compatibility, no built-in TLS, no Unix sockets, no
  status auth, no bind ACLs, and no remote or target port `0`.

## Build

Build for the current platform:

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o rpf .
```

Cross-compile for Linux AMD64:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o rpf-linux-amd64 .
```

## Usage

Start the server:

```bash
RPORT_TOKEN=secret ./rpf server
```

Start a client tunnel:

```bash
RPORT_TOKEN=secret ./rpf client \
  --server 127.0.0.1:9000 \
  --remote 127.0.0.1:8080 \
  --target 127.0.0.1:3000
```

Check server status:

```bash
curl http://127.0.0.1:9001/status
```

## Flags

Server:

```bash
rpf server [--listen :9000] [--status-listen 127.0.0.1:9001] [--open-timeout 10s] [--max-pending 128] [--max-active 1024] [--heartbeat-interval 30s] [--heartbeat-timeout 90s] [--token secret]
```

Client:

```bash
rpf client --server host:port --remote [bind_address:]port --target host:hostport [--token secret] [--reconnect-interval 5s]
```

`--token` overrides `RPORT_TOKEN`. Tokens are required and must not be empty or
contain whitespace.

`--max-pending` and `--max-active` are per-tunnel limits. When a tunnel reaches
capacity, extra remote callers are closed instead of being queued indefinitely.

`--heartbeat-interval` and `--heartbeat-timeout` control server-initiated
control connection heartbeats. If a client does not answer `PING` with `PONG`
before the timeout, the server closes that tunnel and releases its remote
listener.

## Run as backend service

Create a service file in `/lib/systemd/system/reverse-port-forwarding.service`:

```conf
[Unit]
# describe the app
Description=reverse-port-forwarding
# start the app after the network is available
After=network.target

[Service]
# usually you'll use 'simple'
# one of https://www.freedesktop.org/software/systemd/man/systemd.service.html#Type=
Type=simple
# which user to use when starting the app
User=rpf
# path to your application's root directory
WorkingDirectory=/opt/reverse-port
# the command to start the app
# requires absolute paths
Environment="RPORT_TOKEN=change-this-token"
ExecStart=/opt/reverse-port/rpf server --listen :9000 --status-listen 127.0.0.1:9001
KillSignal=SIGTERM
# How long systemd waits for the app to stop before sending SIGKILL
TimeoutStopSec=30s
# Send the stop signal to all processes in the unit's control group
KillMode=control-group
# restart policy
# one of {no|on-success|on-failure|on-abnormal|on-watchdog|on-abort|always}
Restart=always

[Install]
# start the app automatically
WantedBy=multi-user.target
```

## How it works

`rpf` uses two TCP connection roles: **control** and **data**.

```
                         ┌─────── Server ───────┐         ┌─── Client ───┐
                         │                      │         │              │
  remote caller ────────▶│ :8080  (remote)      │  OPEN   │              │
                         │    │                 │────────▶│              │
                         │    ▼                 │         │   ┌──────┐   │
                         │  (pending)           │         │   │target│   │
                         │                      │  DATA   │   │:3000 │   │
                         │ :9000  (tunnel)      │◀────────│   └──▲───┘   │
                         │                      │         │      │       │
                         └──────────────────────┘         └──────┴───────┘
```

Three TCP connections are involved:

- **control** — persistent connection between client and server on the tunnel port
- **data** — one short-lived connection per forwarded request from client back to server
- **target** — client-side connection to the local service

1. **Startup.** The client dials the server's tunnel port and sends a `CONTROL` header
   containing its token, the remote bind address, and the target address. The server
   replies `OK` and starts a TCP listener on the remote address.

2. **Forwarding.** When a remote caller connects to the remote listener, the server
   holds the connection and sends `OPEN <id>` back to the client over the control channel.

3. **Data attach.** The client dials the server again, sends a `DATA` header with its
   token and the connection id, then dials the local target. Once the data connection
   is established, the server pipes the remote caller's traffic to the data connection
   and the client pipes the data connection to the target.

4. **Reconnect.** If the control connection drops, the client retries on a fixed
   interval. No state is preserved between sessions.

## Remote Address Semantics

- `--remote 8080` binds `127.0.0.1:8080`
- `--remote 127.0.0.1:8080` binds loopback explicitly
- `--remote :8080` binds all interfaces
- `--remote '*:8080'` binds all interfaces
- `--remote 0.0.0.0:8080` binds all IPv4 interfaces
- `--remote '[::1]:8080'` uses bracketed IPv6 loopback
- `--remote '[::]:8080'` binds all IPv6 interfaces

Remote and target port `0` are rejected.

## Security Notes

`rpf` authenticates control and data connections with a shared token, but it does
not provide encryption. Use it on trusted networks or wrap it with VPN/TLS when
traffic confidentiality matters.

The status endpoint has no auth in v1 and is intentionally restricted to a
loopback listen address. Status responses do not include tokens or connection
IDs.

## Resources

For N connected clients and M simultaneously active (forwarded) connections:

| Resource | Formula |
|---|---|
| Established TCP connections | N + 3M |
| Server listeners | N + 2 |
| Total goroutines (server + all clients) | 3 + 4N + 8M |

Per-tunnel overhead: 1 persistent TCP connection, 2 server goroutines, 2 client goroutines. Per active forwarded connection: 3 TCP connections and 8 goroutines (4 server-side, 4 client-side).

## Verification

```bash
go test ./...
```
