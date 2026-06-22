# rpf Reverse Port Forwarding Raw Request

## Source

- Date: 2026-06-03
- Source type: user request in Codex chat
- Target future project: `/Users/homerh/Golang/reverse-port`

## Raw Behavior To Implement

The requested behavior is the functionality provided by:

```text
ssh -R [bind_address:]port:host:hostport
```

Only this behavior is wanted:

```text
The behavior specifies that connections to the given TCP port (with bind_address)
on the remote (server) host are to be forwarded to the local side.
This works by allocating a socket to listen to a TCP port on the remote side.
Whenever a connection is made to this port, the connection is forwarded over the
channel, and a connection is made from the local machine to an explicit
destination specified by host and port hostport.
```

## Initial Implementation Request

- Language: Go.
- Code style: simple, minimal, elegant.
- Packaging: one executable.
- CLI shape: two subcommands, `client` and `server`.
- Each subcommand accepts the arguments relevant to that side.

## Later Placement Decision

- The implementation should live in a new separate Go repo/project directory at `/Users/homerh/Golang/reverse-port`.
- The executable/module name is `rpf`.
- `agents-os` should capture the design first.
- Code implementation should happen in the separate repo only after the plan is settled.
