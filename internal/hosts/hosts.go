package hosts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	HostsFileContent     []byte
	OrigHostsFileContent []byte
	Hosts                map[string][]string
)

// NamedResolvers maps canonical shorthand names to upstream DNS server
// addresses. NamedResolverAliases lists shorter aliases that resolve to the
// same canonical name.
var NamedResolvers = map[string]string{
	"cloudflare": "1.1.1.1",
	"google":     "8.8.8.8",
	"quad9":      "9.9.9.9",
	"opendns":    "208.67.222.222",
}

var NamedResolverAliases = map[string]string{
	"cf": "cloudflare",
	"go": "google",
	"q9": "quad9",
	"od": "opendns",
}

// lookupNamedResolver returns the address for a canonical name or alias, if
// either matches (case-insensitively).
func lookupNamedResolver(name string) (string, bool) {
	key := strings.ToLower(name)
	if canonical, ok := NamedResolverAliases[key]; ok {
		key = canonical
	}
	addr, ok := NamedResolvers[key]
	return addr, ok
}

// ReadHostsFile reads the hosts file.
func ReadHostsFile(f string) ([]byte, error) {
	bs, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

// ParseHosts takes in hosts file content and returns a map of parsed results.
func ParseHosts(hostsFileContent []byte) (map[string][]string, error) {
	hostsMap := map[string][]string{}
	for _, line := range strings.Split(strings.Trim(string(hostsFileContent), " \t\r\n"), "\n") {
		line = strings.ReplaceAll(strings.Trim(line, " \t"), "\t", " ")
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}
		pieces := strings.SplitN(line, " ", 2)
		if len(pieces) > 1 && len(pieces[0]) > 0 {
			if names := strings.Fields(pieces[1]); len(names) > 0 {
				hostsMap[pieces[0]] = append(hostsMap[pieces[0]], names...)
			}
		}
	}
	return hostsMap, nil
}

// separateQHosts takes in a hosts file content, and returns the content separated
// into two parts: the qhosts content and the rest of the hosts file content.
// this is done by looking for the qhosts header and footer.
// if the header or footer is not found, it is assumed to be empty
func separateQHosts(hostsFileContent []byte) ([]byte, []byte) {
	var qhosts []byte
	var rest []byte
	var inQHosts bool
	for _, line := range strings.Split(strings.Trim(string(hostsFileContent), " \t\r\n"), "\n") {
		line = strings.ReplaceAll(strings.Trim(line, " \t"), "\t", " ")
		if strings.HasPrefix(line, "# qh start") {
			inQHosts = true
			continue
		}
		if strings.HasPrefix(line, "# qh end") {
			inQHosts = false
			continue
		}
		if inQHosts {
			qhosts = append(qhosts, []byte(line+"\n")...)
		} else {
			rest = append(rest, []byte(line+"\n")...)
		}
	}
	return qhosts, rest
}

func Load(f string) error {
	bd, err := ReadHostsFile(f)
	if err != nil {
		return err
	}
	qhosts, rest := separateQHosts(bd)
	HostsFileContent = qhosts
	OrigHostsFileContent = rest
	OrigHostsFileContent = bytes.TrimRight(OrigHostsFileContent, "\n")
	Hosts, err = ParseHosts(HostsFileContent)
	if err != nil {
		return err
	}
	return nil
}

func Save(f string) error {
	var hostsFileContent []byte
	var hasQH bool
	// sort IPs so save output is deterministic
	ips := make([]string, 0, len(Hosts))
	for ip := range Hosts {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	for _, ip := range ips {
		hasQH = true
		hostsFileContent = append(hostsFileContent, []byte(ip+" "+strings.Join(Hosts[ip], " ")+"\n")...)
	}

	if hasQH {
		hostsFileContent = append([]byte("\n\n# qh start\n"), hostsFileContent...)
		hostsFileContent = append(hostsFileContent, []byte("# qh end\n")...)
		hostsFileContent = append(OrigHostsFileContent, hostsFileContent...)
	} else {
		hostsFileContent = OrigHostsFileContent
	}

	if err := os.WriteFile(f, hostsFileContent, 0644); err != nil {
		return err
	}
	return nil
}

// ReverseLookup takes an IP address and returns a slice of matching hosts file
// entries.
func ReverseLookup(ip string) ([]string, error) {
	return Hosts[ip], nil
}

// Lookup takes a host and returns a slice of matching host file entries
func Lookup(host string) ([]string, error) {
	var hm []string
	for i, h := range Hosts {
		for _, v := range h {
			if strings.EqualFold(v, host) {
				hm = append(hm, i)
			}
		}
	}
	return hm, nil
}

// List returns the qh-managed entries as "<ip> <domain>...\n" lines, sorted
// by IP.
func List() string {
	ips := make([]string, 0, len(Hosts))
	for ip := range Hosts {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	var sb strings.Builder
	for _, ip := range ips {
		sb.WriteString(ip + " " + strings.Join(Hosts[ip], " ") + "\n")
	}
	return sb.String()
}

// Flush removes every qh-managed entry and saves.
func Flush(hostsFile string) error {
	Hosts = map[string][]string{}
	return Save(hostsFile)
}

// ResolveResolverSpec parses a -resolver flag value into a list of host:port
// servers. The spec is comma-separated and each entry may be a NamedResolvers
// key (e.g. "cloudflare"), a bare IP, or "ip:port".
func ResolveResolverSpec(spec string) ([]string, error) {
	if spec == "" {
		return nil, nil
	}
	var out []string
	for _, raw := range strings.Split(spec, ",") {
		s := strings.TrimSpace(raw)
		if s == "" {
			continue
		}
		if v, ok := lookupNamedResolver(s); ok {
			s = v
		}
		if !strings.Contains(s, ":") {
			s += ":53"
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, errors.New("no resolvers parsed from spec")
	}
	return out, nil
}

func resolverFor(server string) *net.Resolver {
	if !strings.Contains(server, ":") {
		server = server + ":53"
	}
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 5 * time.Second}
			return d.DialContext(ctx, network, server)
		},
	}
}

func firstIPv4(ips []net.IP) (string, error) {
	for _, ip := range ips {
		if v4 := ip.To4(); v4 != nil {
			return v4.String(), nil
		}
	}
	return "", errors.New("no A record found")
}

func ipForDomain(domain string, resolvers []string) (string, error) {
	if domain == "localhost" {
		return "127.0.0.1", nil
	}
	if addr := net.ParseIP(domain); addr != nil {
		return domain, nil
	}
	if len(resolvers) == 0 {
		ips, err := net.LookupIP(domain)
		if err != nil {
			return "", err
		}
		return firstIPv4(ips)
	}
	var lastErr error
	for _, server := range resolvers {
		ips, err := resolverFor(server).LookupIP(context.Background(), "ip4", domain)
		if err != nil {
			lastErr = err
			continue
		}
		ip, err := firstIPv4(ips)
		if err != nil {
			lastErr = err
			continue
		}
		return ip, nil
	}
	return "", lastErr
}

func Add(domain, ip string, resolvers []string) error {
	l := log.WithFields(log.Fields{
		"domain": domain,
		"ip":     ip,
	})
	l.Debug("adding domain to hosts file")
	ip, err := ipForDomain(ip, resolvers)
	if err != nil {
		l.WithError(err).Error("failed to resolve ip for domain")
		return err
	}
	l = l.WithField("ip", ip)
	if existing, _ := Lookup(domain); len(existing) > 0 {
		l.Debug("domain already exists in hosts file, removing")
		if err := Remove(domain); err != nil {
			l.WithError(err).Error("failed to remove domain from hosts file")
			return err
		}
	}
	Hosts[ip] = append(Hosts[ip], domain)
	l.Info("added domain to hosts file")
	return nil
}

// Entry pairs a hostname to add with the lookup target (which may itself be
// an IP, a domain to resolve via the system resolver, or a domain to resolve
// via the configured -resolver list).
type Entry struct {
	Domain string
	Target string
}

// AddTemp installs every entry in the hosts file, then waits for either a
// SIGINT or the supplied ttl to elapse (ttl <= 0 means wait indefinitely),
// then removes them again.
func AddTemp(entries []Entry, hostsFile string, resolvers []string, ttl time.Duration) error {
	added := make([]string, 0, len(entries))
	cleanup := func() {
		for _, d := range added {
			if err := RemoveAndSave(d, hostsFile); err != nil {
				log.WithError(err).WithField("domain", d).Error("failed to remove domain from hosts file")
			}
		}
	}
	for _, e := range entries {
		if err := AddAndSave(e.Domain, e.Target, hostsFile, resolvers); err != nil {
			cleanup()
			return err
		}
		added = append(added, e.Domain)
	}

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	defer signal.Stop(sig)

	var timeout <-chan time.Time
	if ttl > 0 {
		t := time.NewTimer(ttl)
		defer t.Stop()
		timeout = t.C
	}

	select {
	case s := <-sig:
		log.WithField("signal", s).Debug("received signal")
	case <-timeout:
		log.WithField("ttl", ttl).Debug("ttl elapsed")
	}

	cleanup()
	return nil
}

func Remove(domain string) error {
	l := log.WithField("domain", domain)
	l.Debug("removing domain from hosts file")
	for ip, domains := range Hosts {
		for i, d := range domains {
			if strings.EqualFold(d, domain) {
				Hosts[ip] = append(domains[:i], domains[i+1:]...)
				l.Info("removed domain from hosts file")
			}
		}
		if len(Hosts[ip]) == 0 {
			delete(Hosts, ip)
		}
	}
	l.Debug("domain not found in hosts file")
	return nil
}

func AddAndSave(domain, ip, hostsFile string, resolvers []string) error {
	if err := Add(domain, ip, resolvers); err != nil {
		return err
	}
	return Save(hostsFile)
}

func RemoveAndSave(domain, hostsFile string) error {
	if err := Remove(domain); err != nil {
		return err
	}
	return Save(hostsFile)
}

// AvailableResolverNames returns the sorted list of named resolver shortcuts.
func AvailableResolverNames() []string {
	names := make([]string, 0, len(NamedResolvers))
	for n := range NamedResolvers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// FormatResolverNames returns "name|alias (ip), name|alias (ip), ..." for
// usage strings, with each canonical name paired with any aliases that point
// to it.
func FormatResolverNames() string {
	aliasesFor := map[string][]string{}
	for alias, canonical := range NamedResolverAliases {
		aliasesFor[canonical] = append(aliasesFor[canonical], alias)
	}
	for _, as := range aliasesFor {
		sort.Strings(as)
	}
	names := AvailableResolverNames()
	parts := make([]string, len(names))
	for i, n := range names {
		label := n
		if as := aliasesFor[n]; len(as) > 0 {
			label = n + "|" + strings.Join(as, "|")
		}
		parts[i] = fmt.Sprintf("%s (%s)", label, NamedResolvers[n])
	}
	return strings.Join(parts, ", ")
}
