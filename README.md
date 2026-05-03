# qh — quick host

A small CLI for adding and removing entries from `/etc/hosts`.

`qh` writes to a clearly-marked block (`# qh start` … `# qh end`) so its
entries are easy to spot, list, and clear, without disturbing the rest of
the file.

## Install

### From a release (recommended)

Prebuilt binaries for darwin / linux / windows on amd64 and arm64 are
attached to every tagged release at
https://github.com/robertlestak/qh/releases.

```bash
# pick the asset for your platform
curl -L -o qh https://github.com/robertlestak/qh/releases/latest/download/qh_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/')

# verify it
curl -L -o qh.sha256 https://github.com/robertlestak/qh/releases/latest/download/qh_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').sha256
shasum -a 256 -c qh.sha256

# install
chmod +x qh
sudo mv qh /usr/local/bin/qh
```

A combined `SHA256SUMS` file covering every asset is published alongside
the per-binary `.sha256` files.

### With go install

```bash
go install github.com/robertlestak/qh/cmd/qh@latest
```

### From source

```bash
make            # builds all six platforms into ./bin
go build -o qh ./cmd/qh   # just for the host platform
```

## Usage

```
qh [flags] <command> [args]
```

Because `/etc/hosts` is owned by root, every command that writes to it must
be run with `sudo`. Read-only commands (`ls`) work without sudo. If you
forget, `qh` will tell you:

```
$ qh add foo.dev 1.2.3.4
qh: open /etc/hosts: permission denied
hint: writing /etc/hosts requires root — try running with sudo
```

`qh` swaps the conventional `/etc/hosts` argument order: the **hostname comes
first**, then the IP (or another hostname to resolve). This matches how you
naturally think about it — "I want `foo.dev` to point at `1.2.3.4`."

If the second argument is itself a hostname, `qh` resolves it with the
system DNS (or the resolver you specify with `-resolver`) and writes the
resulting A record.

### Global flags

| Flag          | Default       | Description                                                                                              |
| ------------- | ------------- | -------------------------------------------------------------------------------------------------------- |
| `-hosts`      | `/etc/hosts`  | Hosts file to edit. Useful for testing.                                                                  |
| `-log-level`  | `info`        | `debug`, `info`, `warn`, `error`, `fatal`.                                                               |
| `-resolver`   | *(none)*      | Comma-separated DNS server(s) to use for hostname resolution. Each may be a name (`cloudflare`) or address (`1.1.1.1`, `1.1.1.1:53`). When set, the IP arg may be omitted — `qh` resolves the domain itself. |
| `-ttl`        | `0`           | For `tmp` only: auto-remove entries after this duration (e.g. `10m`, `2h`). `0` means wait for `Ctrl-C`. |

#### Named resolvers

| Name         | Alias | Address           |
| ------------ | ----- | ----------------- |
| `cloudflare` | `cf`  | `1.1.1.1`         |
| `google`     | `go`  | `8.8.8.8`         |
| `quad9`      | `q9`  | `9.9.9.9`         |
| `opendns`    | `od`  | `208.67.222.222`  |

Aliases are case-insensitive and interchangeable with the full name:
`-resolver cf,go` is the same as `-resolver cloudflare,google`.

When multiple resolvers are given, `qh` tries them in order and uses the
first that returns an answer (failover).

## Commands

### `add` — permanently add one or more hosts

```bash
sudo qh add <domain>... <ip or domain>
sudo qh -resolver <r> add <domain>...
```

```bash
# single
$ sudo qh add mysite.dev 192.168.1.2

# resolve via another hostname
$ sudo qh add mysite.dev example.com

# multiple domains share a single target
$ sudo qh add mysite.dev mysite.local 192.168.1.2

# look each up via Cloudflare DNS
$ sudo qh -resolver cloudflare add mysite.dev mysite.local
```

### `rm` — remove one or more hosts

```bash
sudo qh rm <domain>...
```

```bash
$ sudo qh rm mysite.dev
$ sudo qh rm mysite.dev mysite.local
```

### `tmp` — install temporarily, then auto-clean

`tmp` adds the entries, blocks until `Ctrl-C` (or `-ttl` elapses), then
removes them. Useful for one-off testing without leaving stale lines in
your hosts file.

```bash
sudo qh tmp <domain>... <ip or domain>
sudo qh -resolver <r> tmp <domain>...
sudo qh -ttl <duration> tmp <domain>... <ip or domain>
```

```bash
# Ctrl-C to clean up
$ sudo qh tmp mysite.dev example.com
^C

# auto-clean after 10 minutes
$ sudo qh -ttl 10m tmp mysite.dev 192.168.1.2

# the original use case: my work DNS blocks tracker domains, but I
# need to reach one for debugging — use Cloudflare for the lookup
$ sudo qh -resolver 1.1.1.1 tmp some-blocked-domain.example
```

The bare form `sudo qh <domain> <ip>` (no command word) is an alias for
`tmp`, kept for compatibility.

### `ls` — list current qh-managed entries

Read-only; no sudo needed.

```bash
$ qh ls
1.2.3.4 mysite.dev mysite.local
9.9.9.9 throwaway.example
```

### `flush` — remove every qh-managed entry

```bash
$ sudo qh flush
```

This clears the entire `# qh start` … `# qh end` block. Anything else in
your hosts file is untouched.

## How `qh` stores its entries

`qh` keeps its entries inside a marked block at the end of `/etc/hosts`:

```
127.0.0.1 localhost
255.255.255.255 broadcasthost
::1            localhost


# qh start
1.2.3.4 mysite.dev mysite.local
9.9.9.9 throwaway.example
# qh end
```

Lines outside this block are never touched. The block is sorted by IP for
deterministic diffs, and is removed entirely once the last `qh` entry is
gone.

## Development

```bash
go build ./...          # build
go test ./... -cover    # run tests
go vet ./...
```

The hosts package has tests covering parsing, save/load round-trips,
add/remove/flush, the resolver-spec parser, custom-resolver lookups
(against an in-process mock DNS server), resolver fallover, and TTL-based
auto-cleanup.
