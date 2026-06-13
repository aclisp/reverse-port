# rpf

`rpf` is a small reverse TCP port forwarding tool with behavior similar to:

```text
ssh -R [bind_address:]port:host:hostport
```

It uses a custom minimal protocol, not SSH. The v1 implementation is TCP-only,
standard-library only, and supports one reverse forward per client process.

## Usage

Start the server:

```bash
RPORT_TOKEN=secret go run . server
```

Start a client tunnel:

```bash
RPORT_TOKEN=secret go run . client \
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
# Ensures systemd only considers the main process for the stop signal
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
| Total goroutines (server + all clients) | 2 + 4N + 8M |

Per-tunnel overhead: 1 persistent TCP connection, 2 server goroutines, 2 client goroutines. Per active forwarded connection: 3 TCP connections and 8 goroutines (4 server-side, 4 client-side).

## Verification

```bash
go test ./...
```
