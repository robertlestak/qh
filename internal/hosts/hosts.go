package hosts

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net"
	"os"
	"os/signal"
	"strings"

	log "github.com/sirupsen/logrus"
)

var (
	HostsFileContent     []byte
	OrigHostsFileContent []byte
	Hosts                map[string][]string
)

// ReadHostsFile reads the hosts file.
func ReadHostsFile(f string) ([]byte, error) {
	bs, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}
	return bs, nil
}

// ParseHosts takes in hosts file content and returns a map of parsed results.
func ParseHosts(hostsFileContent []byte) (map[string][]string, error) {
	hostsMap := map[string][]string{}
	for _, line := range strings.Split(strings.Trim(string(hostsFileContent), " \t\r\n"), "\n") {
		line = strings.Replace(strings.Trim(line, " \t"), "\t", " ", -1)
		if len(line) == 0 || line[0] == ';' || line[0] == '#' {
			continue
		}
		pieces := strings.SplitN(line, " ", 2)
		if len(pieces) > 1 && len(pieces[0]) > 0 {
			if names := strings.Fields(pieces[1]); len(names) > 0 {
				if _, ok := hostsMap[pieces[0]]; ok {
					hostsMap[pieces[0]] = append(hostsMap[pieces[0]], names...)
				} else {
					hostsMap[pieces[0]] = names
				}
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
		line = strings.Replace(strings.Trim(line, " \t"), "\t", " ", -1)
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
	// remove the trailing newlines from OrigHostsFileContent
	OrigHostsFileContent = bytes.TrimRight(OrigHostsFileContent, "\n")
	Hosts, err = ParseHosts(HostsFileContent)
	if err != nil {
		return err
	}
	return nil
}

func Save(f string) error {
	// write the hosts map to the hosts file
	var hostsFileContent []byte
	var hasQH bool
	for ip, domains := range Hosts {
		hasQH = true
		hostsFileContent = append(hostsFileContent, []byte(ip+" "+strings.Join(domains, " ")+"\n")...)
	}

	// add header and footer
	if hasQH {
		hostsFileContent = append([]byte("\n\n# qh start\n"), hostsFileContent...)
		hostsFileContent = append(hostsFileContent, []byte("# qh end\n")...)

		hostsFileContent = append(OrigHostsFileContent, hostsFileContent...)
	} else {
		hostsFileContent = OrigHostsFileContent
	}

	if err := ioutil.WriteFile(f, hostsFileContent, 0644); err != nil {
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

func ipForDomain(domain string) (string, error) {
	if domain == "localhost" {
		return "127.0.0.1", nil
	}
	addr := net.ParseIP(domain)
	if addr != nil {
		return domain, nil
	} else {
		ips, err := net.LookupIP(domain)
		if err != nil {
			return "", err
		}
		for _, ip := range ips {
			if ip.To4() != nil {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("no A record found")
}

func Add(domain, ip string) error {
	l := log.WithFields(log.Fields{
		"domain": domain,
		"ip":     ip,
	})
	l.Debug("adding domain to hosts file")
	ip, err := ipForDomain(ip)
	if err != nil {
		l.WithError(err).Error("failed to resolve ip for domain")
		return err
	}
	l = l.WithField("ip", ip)
	// check if domain already exists, if so, overwrite
	if _, err := Lookup(domain); err == nil {
		l.Debug("domain already exists in hosts file, removing")
		if err := Remove(domain); err != nil {
			l.WithError(err).Error("failed to remove domain from hosts file")
			return err
		}
	}
	// add domain to hosts map
	if _, ok := Hosts[ip]; ok {
		Hosts[ip] = append(Hosts[ip], domain)
	} else {
		Hosts[ip] = []string{domain}
	}
	l.Info("added domain to hosts file")
	return nil
}

func AddTemp(domain, ip, hostsFile string) error {
	l := log.WithFields(log.Fields{
		"domain": domain,
		"ip":     ip,
	})
	l.Debug("adding domain to hosts file")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	if err := AddAndSave(domain, ip, hostsFile); err != nil {
		l.WithError(err).Error("failed to add domain to hosts file")
		return err
	}
	l.Debug("added domain to hosts file")
	for sig := range c {
		l.WithField("signal", sig).Debug("received signal")
		if err := RemoveAndSave(domain, hostsFile); err != nil {
			l.WithError(err).Error("failed to remove domain from hosts file")
		}
		os.Exit(1)
	}
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
		// if there are no more domains for this ip, remove the ip
		if len(Hosts[ip]) == 0 {
			delete(Hosts, ip)
		}
	}
	l.Debug("domain not found in hosts file")
	return nil
}

func AddAndSave(domain, ip, hostsFile string) error {
	if err := Add(domain, ip); err != nil {
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
