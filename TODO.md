# TODO

- Deployment risk: token auth is not encryption. `rpf` sends protocol headers and forwarded TCP traffic in cleartext, so production use on untrusted networks should wrap the tunnel with VPN/TLS or add built-in transport encryption in a future version.
- Client `handleOpen` goroutines are unbounded. Each server OPEN spawns a goroutine that holds two TCP connections until `pipeBidirectional` completes. Under extreme load this could exhaust file descriptors or goroutine stack memory on the client host. A per-tunnel semaphore (e.g. `semaphore.Weighted`) would cap concurrency if this becomes an issue in practice.
