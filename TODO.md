# TODO

- Deployment risk: token auth is not encryption. `rpf` sends protocol headers and forwarded TCP traffic in cleartext, so production use on untrusted networks should wrap the tunnel with VPN/TLS or add built-in transport encryption in a future version.
- TOCTOU race in `t.run()`: `canAcceptRemote()` and `t.pending[id] = pc` use separate lock acquisitions, so concurrent remote connections can exceed `maxPendingConnections`.
- TOCTOU race in `handleDataConn()`: `canActivate()` and `addActive()` use separate lock acquisitions, so concurrent data connections can exceed `maxActiveConnections`.
