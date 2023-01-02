# qh - quick host

A simple tool to quickly add and remove hosts from your `/etc/hosts` file.

## Usage

Since `/etc/hosts` is a protected file, you need to run `qh` with `sudo`.

While `/etc/hosts` syntax is `ip hostname`, `qh` uses `hostname ip` to make it easier to remember, since the hostname is the one you'll be using most of the time. Additionally, while `/etc/hosts` requires a single A IP, `qh` allows you to use a hostname as well. If a hostname is provided, it will be resolved to an IP address before being added to the `/etc/hosts` file.

### Add a host

```bash
$ sudo qh add [domain] [ip or hostname]
```

#### Example

```bash
$ sudo qh add mysite.dev 192.168.1.2
$ sudo qh add mysite2.dev example.com
```

### Remove a host

```bash
$ sudo qh rm [domain]
```

#### Example

```bash
$ sudo qh rm mysite.dev
```

### Create a temporary host

```bash
$ sudo qh tmp [domain] [ip or hostname]
```

#### Example

```bash
$ sudo qh tmp mysite.dev example.com
...
^C
```

This will add the host to your `/etc/hosts` file, but will remove it when you exit the process (Ctrl+C).