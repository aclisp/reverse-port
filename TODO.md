# TODO

- Deployment risk: token auth is not encryption. `rpf` sends protocol headers and forwarded TCP traffic in cleartext, so production use on untrusted networks should wrap the tunnel with VPN/TLS or add built-in transport encryption in a future version.
- Fix the server shutdown lifecycle issue where context cancellation closes the main tunnel listener and status server but does not explicitly close existing per-client remote listeners and active tunnels.
- Add an application heartbeat on the control connection so stale or half-open client connections are detected within a bounded timeout and stale remote listeners are released.
- TOCTOU race in `t.run()`: `canAcceptRemote()` and `t.pending[id] = pc` use separate lock acquisitions, so concurrent remote connections can exceed `maxPendingConnections`.
- TOCTOU race in `handleDataConn()`: `canActivate()` and `addActive()` use separate lock acquisitions, so concurrent data connections can exceed `maxActiveConnections`.
