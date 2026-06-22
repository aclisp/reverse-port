# rpf Reverse Port Forwarding Design Clarification

## Context

- Target future project: `/Users/homerh/Golang/reverse-port`
- Source input: `docs/input/2026-06-03-rpf-reverse-port-forwarding-request.md`
- Requested method: grill the plan one question at a time before implementation.
- Task route: requirement clarification and architecture/design capture for a new external Go executable.
- Skill used: external `grill-me` skill.

## Confirmed Decisions

| # | Decision |
|---|----------|
| 1 | Implement a custom minimal tunnel protocol, not SSH protocol compatibility. |
| 2 | Use shared secret token authentication. Built-in encryption/TLS is out of scope for v1. |
| 3 | The client sends the requested remote bind address, so the server remains general and reusable. |
| 4 | v1 supports one reverse forward per client process. |
| 5 | Remote bind failures are reported clearly. After the reconnect decision, bind failures are retried rather than causing permanent client exit. |
| 6 | One server supports multiple simultaneous clients if their requested remote listeners do not conflict. |
| 7 | Use one persistent control connection plus one short-lived data connection per inbound remote TCP connection. |
| 8 | Use one server tunnel listen port for both control and data connections. The initial protocol header distinguishes connection type. |
| 9 | Use a simple line-oriented text protocol for headers, then raw byte piping after valid data attach. |
| 10 | If the client cannot dial the local target for an inbound connection, it still opens the matching data connection and closes it immediately so the server can close the remote caller promptly. |
| 11 | Server open wait timeout defaults to 10s and is configurable with `--open-timeout`. |
| 12 | When a client control connection disconnects, the server tears down that client's remote listener, pending opens, and active forwarded connections. |
| 13 | Remote bind semantics mirror OpenSSH: omitted bind address defaults to loopback; empty bind address or `*` means all interfaces. |
| 14 | v1 is TCP only. Unix sockets are out of scope. |
| 15 | The client dials a fresh local target TCP connection for each inbound remote connection. |
| 16 | IPv6 is supported through normal bracketed address syntax. |
| 17 | Use only the Go standard library. |
| 18 | Build one binary in one Go package, split across several small files. |
| 19 | Server and client both support clean SIGINT/SIGTERM cancellation. |
| 20 | Use minimal operational logs to stderr only. No verbose mode in v1. |
| 21 | Use one shared server token for all clients. |
| 22 | v1 has no bind ACLs beyond token authentication and OS permissions. |
| 23 | Control connection loss immediately closes active forwarded connections. |
| 24 | The client should reconnect while the client process remains alive. |
| 25 | Reconnect uses a fixed interval, not exponential backoff. |
| 26 | Reconnect interval defaults to 5s and is configurable with `--reconnect-interval`. |
| 27 | Initial and later remote bind failures retry forever. Invalid local config exits immediately. |
| 28 | Remote port `0` is not supported in v1. |
| 29 | Target port `0` is rejected during argument validation. |
| 30 | The client does not preflight the target at startup. It dials the target only per inbound connection. |
| 31 | `--target` allows hostnames. |
| 32 | `--server` allows hostnames. |
| 33 | `--remote` allows hostnames as server-side bind names passed to `net.Listen`. |
| 34 | Enable TCP keepalive on accepted and outbound TCP connections where available. |
| 35 | Use constant-time token comparison. |
| 36 | Token may come from `--token` or `RPORT_TOKEN`; CLI flag takes precedence. |
| 37 | Executable/module name is `rpf`. |
| 38 | Subcommands are exactly `server` and `client`, with no aliases in v1. |
| 39 | Implementation lives in a new separate repo/project directory at `/Users/homerh/Golang/reverse-port`; executable/module name is `rpf`; `agents-os` captures design first. |
| 40 | Each control connection owns exactly one remote listener. |
| 41 | Connection IDs are server-generated random hex strings from `crypto/rand`. |
| 42 | Data sockets identify by token and connection ID only. |
| 43 | Unknown, expired, or already-used data connection IDs are rejected and closed immediately. |
| 44 | Pending connection IDs are single-use. |
| 45 | Valid data attach switches directly to raw piping with no response line. |
| 46 | Bidirectional piping should be half-close aware when TCP supports it. |
| 47 | v1 has no idle data deadlines for active forwarded streams. |
| 48 | Control errors are detailed enough to debug; data attach errors are minimal. |
| 49 | Tokens must be non-empty and contain no whitespace. |
| 50 | Address arguments must contain no whitespace. |
| 51 | Both client and server validate `--remote`. |
| 52 | `--remote` and `--target` remain separate flags. No combined `remote:target` form. |
| 53 | Protocol-prefixed addresses such as `tcp://...` are rejected. |
| 54 | Privileged-port permission errors are treated as normal bind failures and retried. |
| 55 | `rpf server --listen` defaults to `:9000`. |
| 56 | `rpf client --server` is required. |
| 57 | `rpf client --remote` and `--target` are required. |
| 58 | `rpf server` requires a token via `--token` or `RPORT_TOKEN`. |
| 59 | Invalid CLI usage prints subcommand usage and exits with code `2`. |
| 60 | Server logs distinguish authentication failure from malformed requests. |
| 61 | Server includes a status HTTP endpoint in v1. |
| 62 | Status HTTP listener is separate from the tunnel listener and defaults to `127.0.0.1:9001`. |
| 63 | Status exposes JSON at `GET /status` with counts and active tunnel summaries. |
| 64 | Status endpoint is unauthenticated because v1 only supports loopback binding. |
| 65 | `--status-listen` may change the status port/address only when the address is loopback. |
| 66 | Status is enabled by default and server startup fails if it cannot bind. |
| 67 | Status includes current counts plus simple cumulative totals. |
| 68 | v1 has no custom client identity; status uses observed client socket address. |
| 69 | The client sends target address in the control header so the server can display it in status. |
| 70 | Status may include target address, but never token or protocol secrets. |

## Resolved CLI Shape

```bash
rpf server \
  --listen :9000 \
  --status-listen 127.0.0.1:9001 \
  --open-timeout 10s \
  --token secret

rpf client \
  --server server.example:9000 \
  --remote 127.0.0.1:8080 \
  --target 127.0.0.1:3000 \
  --token secret \
  --reconnect-interval 5s
```

`RPORT_TOKEN` may replace `--token`; `--token` wins when both are set.

## Open Questions

- No known blocking product questions remain from the design clarification.
- Plan audit is still required before implementation starts.
