# TODO

- Deployment risk: token auth is not encryption. `rpf` sends protocol headers and forwarded TCP traffic in cleartext, so production use on untrusted networks should wrap the tunnel with VPN/TLS or add built-in transport encryption in a future version.
