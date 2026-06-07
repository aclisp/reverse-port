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
rpf server [--listen :9000] [--status-listen 127.0.0.1:9001] [--open-timeout 10s] [--max-pending 128] [--max-active 1024] [--token secret]
```

Client:

```bash
rpf client --server host:port --remote [bind_address:]port --target host:hostport [--token secret] [--reconnect-interval 5s]
```

`--token` overrides `RPORT_TOKEN`. Tokens are required and must not be empty or
contain whitespace.

`--max-pending` and `--max-active` are per-tunnel limits. When a tunnel reaches
capacity, extra remote callers are closed instead of being queued indefinitely.

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

## Verification

```bash
go test ./...
```
