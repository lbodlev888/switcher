# switcher

A minimal TCP relay that does what SOCKS5 does — let a client reach a remote service through an intermediary host — but **without requiring the application to know anything about the protocol**.

Where SOCKS5 needs the client app to speak SOCKS5 (and many apps don't), `switcher` opens a plain TCP listener on `localhost`. The application connects to it as if it were the real service. The local `switcher` client forwards everything to a `switcher` server on another host, the server dials the real destination, and bytes are relayed in both directions for the lifetime of the connection.

It's best suited for **long-lived, stream-oriented traffic** (SSH, databases, custom binary protocols, video streams). It is not optimized for one-shot request/response traffic like DNS lookups, since each tunnel currently uses a single dedicated TCP connection from end to end.

---

## How it works

```
   ┌─────────────┐        ┌──────────────────┐         ┌───────────────────┐         ┌──────────────┐
   │ application │──TCP──▶│ switcher client  │──TCP───▶│ switcher server   │──TCP───▶│ destination  │
   │ (unmodified)│        │ (local listener) │         │ (on remote host)  │         │ service      │
   └─────────────┘        └──────────────────┘         └───────────────────┘         └──────────────┘
```

1. The **client** binds a random local TCP port (`:0`) and prints the address it picked. The user points the unmodified application at that address.
2. When the application connects, the client dials the configured **server** address.
3. The client sends a **6-byte destination header** describing where the server should connect.
4. The server decodes the header, dials the destination, and writes back a 1-byte status code.
5. On success, both sides enter a bidirectional `io.Copy` loop until either end closes.

### Wire protocol

The control handshake is tiny — there is no version byte, no authentication, no negotiation.

**Client → Server (6 bytes, exactly once per connection):**

| Offset | Size | Field   | Encoding              |
|-------:|-----:|---------|-----------------------|
|      0 |    4 | IPv4    | raw bytes (`net.IP.To4()`) |
|      4 |    2 | Port    | uint16, big-endian    |

**Server → Client (1 byte):**

| Value  | Meaning                                                      |
|--------|--------------------------------------------------------------|
| `0x01` | Dial succeeded; relay begins                                 |
| `0x02` | Failed to read the destination header from the client        |
| `0x03` | Header was the wrong length (expected exactly 6 bytes)       |
| `0x04` | Header decoded but was invalid (e.g. malformed IPv4)         |
| `0x05` | Dial to the destination failed (refused, unreachable, etc.)  |

Any non-`0x01` value is fatal for the connection — the server logs the underlying error, writes the code, and closes.

After the status byte, the connection is a transparent byte pipe — no framing, no length prefixes, no multiplexing.

---

## Tech stack

- **Language:** Go (`go 1.26.3`, see `go.mod`)
- **Dependencies:** standard library `net`, `io`, `context`, `flag`, `os/signal` only. `github.com/hashicorp/yamux` is listed as an indirect dependency, left over from an earlier multiplexed design that was reverted (see commit `113e9a3`); it is not used by the current code and can be removed with `go mod tidy`.
- **Layout:**
  - `main.go` — entry point and flag parsing; dispatches to client or server mode.
  - `client/main.go` — local listener, handshake, relay.
  - `server/main.go` — accept loop, handshake, dial, relay.
  - `network/transport.go` — `Relay` (bidirectional `io.Copy`) and the `EncodeDestination` / `DecodeDestination` helpers for the 6-byte header.

The whole codebase is under ~200 lines of Go.

---

## Building

```sh
go build -o switcher .
```

This produces a single static binary (~3.7 MB) that contains **both** client and server roles, switched by a flag.

---

## Usage

### Run the server (on the host that has network access to the real destination)

```sh
./switcher -server -listen :3001
```

Flags:
- `-server` — run in server mode.
- `-listen` — address to bind. Default `:3001`.

### Run the client (on the host running the application)

```sh
./switcher -switch <server_host>:3001 -dest <destination_ip>:<destination_port>
```

Flags:
- `-switch` — `host:port` of the running switcher server.
- `-dest` — IPv4 destination the server should dial, e.g. `10.0.0.5:5432`.

The client logs the local address it bound, for example:

```
Local listener started on 127.0.0.1:39481
Waiting for local interaction...
```

Point your application at `127.0.0.1:39481` and it will reach the destination through the server.

### Example: tunneling Postgres

Server (sitting in the network that can reach the database):
```sh
./switcher -server -listen :3001
```

Client (your laptop):
```sh
./switcher -switch jumphost.example.com:3001 -dest 10.0.0.5:5432
# → Local listener started on 127.0.0.1:39481
psql -h 127.0.0.1 -p 39481 -U app appdb
```

---

## Current design choices & limitations

These are deliberate (the project is intentionally minimal) but worth knowing before using it for anything serious.

- **One tunnel per client invocation.** `client.RunClient` calls `listener.Accept()` exactly once, services that connection, and exits. To handle several application connections, run several clients (each gets its own random local port), or wrap the call in a loop.
- **IPv4 only.** `EncodeDestination` calls `.To4()` and rejects anything that doesn't fit in four bytes. No hostname resolution either — the client must already know the destination IP.
- **No authentication, no encryption.** The control header and the relayed traffic are sent in cleartext over plain TCP. Use it only on trusted networks, or front it with TLS / WireGuard / SSH.
- **No connection multiplexing.** Each tunnel is its own end-to-end TCP connection (the earlier yamux-based multiplexed design was reverted in `113e9a3` in favor of N independent connections).
- **No timeouts.** Reads and writes block indefinitely; the relay only ends when one peer closes or `SIGINT` is received.
- **Status codes are server-side only.** The server reports which handshake step failed (`0x02`–`0x05`), but the current client only checks for `0x01` and logs the raw byte otherwise — it does not map the code to a human-readable cause.

---

## Graceful shutdown

Both modes install a `signal.NotifyContext(ctx, os.Interrupt)` handler. On `Ctrl-C` the context is cancelled, listeners and live connections are closed, and `Relay` unblocks because both copy goroutines see EOF. The server uses a `sync.WaitGroup` to wait for in-flight handlers before exiting.

---

## Possible next steps

If you want to extend this, the obvious directions are:
- Loop the client's `Accept` so one invocation handles many app connections.
- Restore multiplexing (yamux is still in `go.sum`) so multiple app connections share one TCP connection to the server — useful over high-latency links.
- Extend the header with an address-type byte (à la SOCKS5) to support IPv6 and DNS names.
- Wrap the client↔server hop in TLS, and add a shared-secret or mTLS handshake.
- Add idle/read deadlines so dead tunnels don't linger.
