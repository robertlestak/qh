package main

import (
	"flag"
	"fmt"

	"github.com/robertlestak/qh/internal/hosts"
	log "github.com/sirupsen/logrus"
)

func printUsage() {
	fmt.Println("usage: qh [flags] <command> [args]")
	fmt.Println("flags:")
	flag.PrintDefaults()
	fmt.Println("commands:")
	fmt.Println("  add <domain> <ip or domain>")
	fmt.Println("  rm <domain>")
	fmt.Println("  tmp <domain> <ip or domain>")
	fmt.Println("  <domain> <ip or domain>")
}

func main() {
	var hostsFile string
	var logLevel string
	flag.StringVar(&hostsFile, "hosts", "/etc/hosts", "hosts file to use")
	flag.StringVar(&logLevel, "log-level", "info", "log level")
	flag.Parse()
	if err := hosts.Load(hostsFile); err != nil {
		log.Fatal(err)
	}
	args := flag.Args()
	ll, err := log.ParseLevel(logLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(ll)
	if len(args) == 0 {
		printUsage()
		return
	}
	var cmd string
	cmd, args = args[0], args[1:]
	switch cmd {
	case "add":
		if len(args) != 2 {
			log.Fatal("usage: qh add <domain> <ip or domain>")
		}
		domain, ip := args[0], args[1]
		if err := hosts.AddAndSave(domain, ip, hostsFile); err != nil {
			log.Fatal(err)
		}
	case "rm", "remove":
		if len(args) != 1 {
			log.Fatal("usage: qh rm <domain>")
		}
		domain := args[0]
		if err := hosts.RemoveAndSave(domain, hostsFile); err != nil {
			log.Fatal(err)
		}
	case "tmp", "temp":
		if len(args) != 2 {
			log.Fatal("usage: qh tmp <domain> <ip or domain>")
		}
		domain, ip := args[0], args[1]
		if err := hosts.AddTemp(domain, ip, hostsFile); err != nil {
			log.Fatal(err)
		}
	default:
		if len(args) != 1 {
			log.Fatal("usage: qh <domain> <ip or domain>")
		}
		domain, ip := cmd, args[0]
		if err := hosts.AddTemp(domain, ip, hostsFile); err != nil {
			log.Fatal(err)
		}
	}
}
