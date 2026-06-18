# Resource Usage

For N connected clients and M simultaneously active (forwarded) connections.

## TCP Connections (steady state)

| Connection | Direction | Count |
|---|---|---|
| Control | client → server (tunnel port) | N |
| Remote | caller → server (remote listener) | M |
| Data | client → server (tunnel port) | M |
| Target | client → local service | M |
| **Total** | | **N + 3M** |

Plus N + 2 listening sockets on the server: 1 tunnel listener, 1 status listener, and N remote listeners (one per tunnel).

## Goroutines

### Server side

| Component | Count |
|---|---|
| Fixed infrastructure (tunnel listener accept loop, ctx watcher, HTTP serve) | 3 |
| Per tunnel: `handleServerConn` accept loop | N |
| Per tunnel: `monitorControl` | N |
| Per active connection: `handleDataConn` (blocked in `pipeBidirectional`) | M |
| Per active connection: 2x `pipeOneWay` + 1x `wg.Wait` closer | 3M |
| **Total** | **3 + 2N + 4M** |

### Client side (across all N client processes)

| Component | Count |
|---|---|
| Per client: reconnect loop (main goroutine) | N |
| Per client: context watcher in `runClientSession` | N |
| Per active connection: `handleOpen` (blocked in `pipeBidirectional`) | M |
| Per active connection: 2x `pipeOneWay` + 1x `wg.Wait` closer | 3M |
| **Total** | **2N + 4M** |

### Grand total goroutines: 3 + 4N + 8M

## Per-unit cost

- Per tunnel: 1 TCP connection, 4 goroutines (2 server, 2 client)
- Per active forwarded connection: 3 TCP connections, 8 goroutines (4 server, 4 client)
