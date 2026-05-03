package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"net"
	"os"
	"time"

	"github.com/robertlestak/qh/internal/hosts"
	log "github.com/sirupsen/logrus"
)

func printUsage() {
	fmt.Println("usage: qh [flags] <command> [args]")
	fmt.Println("flags:")
	flag.PrintDefaults()
	fmt.Println("commands:")
	fmt.Println("  add <domain>... <ip or domain>")
	fmt.Println("  add -resolver <r> <domain>...")
	fmt.Println("  rm <domain>...")
	fmt.Println("  tmp <domain>... <ip or domain>")
	fmt.Println("  tmp -resolver <r> <domain>...")
	fmt.Println("  ls")
	fmt.Println("  flush")
	fmt.Println("  <domain>... <ip or domain>")
	fmt.Println("named resolvers: " + hosts.FormatResolverNames())
}

// splitDomainsAndTarget partitions positional args based on whether a resolver
// is configured. With a resolver: every arg is a domain, each looked up
// independently. Without: the last arg is the IP/host target shared by all
// preceding domains.
func splitDomainsAndTarget(name string, args []string, resolverSpec string) []hosts.Entry {
	if len(args) == 0 {
		log.Fatalf("usage: qh %s <domain>... <ip or domain>", name)
	}
	if resolverSpec != "" {
		entries := make([]hosts.Entry, len(args))
		for i, d := range args {
			entries[i] = hosts.Entry{Domain: d, Target: d}
		}
		return entries
	}
	if len(args) < 2 {
		// One arg, no resolver — ambiguous. Treat it as a domain that resolves
		// to itself via the system DNS, matching prior single-domain behavior.
		return []hosts.Entry{{Domain: args[0], Target: args[0]}}
	}
	target := args[len(args)-1]
	domains := args[:len(args)-1]
	entries := make([]hosts.Entry, len(domains))
	for i, d := range domains {
		entries[i] = hosts.Entry{Domain: d, Target: target}
	}
	return entries
}

func looksLikeDomain(s string) bool {
	// Used only to validate that, when -resolver is set, the user didn't
	// accidentally pass a literal IP as a domain.
	return net.ParseIP(s) == nil
}

func handleErr(err error) {
	if err == nil {
		return
	}
	if errors.Is(err, fs.ErrPermission) {
		fmt.Fprintf(os.Stderr, "qh: %v\nhint: writing /etc/hosts requires root — try running with sudo\n", err)
		os.Exit(1)
	}
	log.Fatal(err)
}

func main() {
	var hostsFile string
	var logLevel string
	var resolverSpec string
	var ttl time.Duration
	flag.StringVar(&hostsFile, "hosts", "/etc/hosts", "hosts file to use")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.StringVar(&resolverSpec, "resolver", "", "comma-separated DNS server(s) to resolve against (e.g. 1.1.1.1,8.8.8.8 or names like cloudflare,google). when set, the ip arg may be omitted")
	flag.DurationVar(&ttl, "ttl", 0, "for tmp: auto-remove the entries after this duration (e.g. 10m). 0 = wait for SIGINT")
	flag.Parse()

	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(ll)

	resolvers, err := hosts.ResolveResolverSpec(resolverSpec)
	if err != nil {
		log.Fatal(err)
	}

	if err := hosts.Load(hostsFile); err != nil {
		handleErr(err)
	}

	args := flag.Args()
	if len(args) == 0 {
		printUsage()
		return
	}
	cmd, args := args[0], args[1:]

	switch cmd {
	case "add":
		entries := splitDomainsAndTarget("add", args, resolverSpec)
		if resolverSpec != "" {
			for _, e := range entries {
				if !looksLikeDomain(e.Domain) {
					log.Fatalf("with -resolver, all positional args must be domains; got literal IP %q", e.Domain)
				}
			}
		}
		for _, e := range entries {
			handleErr(hosts.AddAndSave(e.Domain, e.Target, hostsFile, resolvers))
		}
	case "rm", "remove":
		if len(args) == 0 {
			log.Fatal("usage: qh rm <domain>...")
		}
		for _, d := range args {
			handleErr(hosts.RemoveAndSave(d, hostsFile))
		}
	case "tmp", "temp":
		entries := splitDomainsAndTarget("tmp", args, resolverSpec)
		handleErr(hosts.AddTemp(entries, hostsFile, resolvers, ttl))
	case "ls", "list":
		out := hosts.List()
		if out == "" {
			fmt.Println("(no qh-managed entries)")
			return
		}
		fmt.Print(out)
	case "flush":
		handleErr(hosts.Flush(hostsFile))
	default:
		all := append([]string{cmd}, args...)
		entries := splitDomainsAndTarget(cmd, all, resolverSpec)
		// When no resolver and only one arg was given, splitDomainsAndTarget
		// produced (cmd -> cmd) — that's not what the bare form means. Reject.
		if resolverSpec == "" && len(all) < 2 {
			fmt.Fprintln(os.Stderr, "usage: qh <domain>... <ip or domain>")
			os.Exit(1)
		}
		handleErr(hosts.AddTemp(entries, hostsFile, resolvers, ttl))
	}
}
