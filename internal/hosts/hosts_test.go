package hosts

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

func init() {
	log.SetLevel(log.PanicLevel)
}

// reset wipes package-level globals between tests.
func reset() {
	Hosts = map[string][]string{}
	HostsFileContent = nil
	OrigHostsFileContent = nil
}

func writeHosts(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "hosts")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func readHosts(t *testing.T, p string) string {
	t.Helper()
	bs, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(bs)
}

func TestParseHosts(t *testing.T) {
	in := []byte("# comment\n127.0.0.1 localhost loop\n; another comment\n10.0.0.1\tone\ttwo\n\n")
	got, err := ParseHosts(in)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"localhost", "loop"}; !equalSlices(got["127.0.0.1"], want) {
		t.Errorf("127.0.0.1 = %v, want %v", got["127.0.0.1"], want)
	}
	if want := []string{"one", "two"}; !equalSlices(got["10.0.0.1"], want) {
		t.Errorf("10.0.0.1 = %v, want %v", got["10.0.0.1"], want)
	}
}

func TestSeparateQHosts(t *testing.T) {
	in := []byte("127.0.0.1 localhost\n\n# qh start\n1.2.3.4 example.com\n# qh end\n255.255.255.255 broadcasthost\n")
	qh, rest := separateQHosts(in)
	if !strings.Contains(string(qh), "1.2.3.4 example.com") {
		t.Errorf("qh section missing entry: %q", qh)
	}
	if strings.Contains(string(qh), "localhost") || strings.Contains(string(qh), "broadcasthost") {
		t.Errorf("qh section leaked non-qh entries: %q", qh)
	}
	if !strings.Contains(string(rest), "127.0.0.1 localhost") || !strings.Contains(string(rest), "broadcasthost") {
		t.Errorf("rest missing original entries: %q", rest)
	}
	if strings.Contains(string(rest), "example.com") {
		t.Errorf("rest leaked qh entry: %q", rest)
	}
}

func TestLoadSaveRoundTrip(t *testing.T) {
	reset()
	orig := "127.0.0.1 localhost\n255.255.255.255 broadcasthost\n\n# qh start\n1.2.3.4 example.com\n# qh end\n"
	p := writeHosts(t, orig)
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if got := Hosts["1.2.3.4"]; !equalSlices(got, []string{"example.com"}) {
		t.Errorf("Hosts[1.2.3.4] = %v, want [example.com]", got)
	}
	if err := Save(p); err != nil {
		t.Fatal(err)
	}
	out := readHosts(t, p)
	if !strings.Contains(out, "127.0.0.1 localhost") {
		t.Errorf("save dropped original entry: %q", out)
	}
	if !strings.Contains(out, "1.2.3.4 example.com") {
		t.Errorf("save dropped qh entry: %q", out)
	}
	if !strings.Contains(out, "# qh start") || !strings.Contains(out, "# qh end") {
		t.Errorf("save dropped qh markers: %q", out)
	}
}

func TestSaveWithoutQHEntriesOmitsMarkers(t *testing.T) {
	reset()
	orig := "127.0.0.1 localhost\n"
	p := writeHosts(t, orig)
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := Save(p); err != nil {
		t.Fatal(err)
	}
	out := readHosts(t, p)
	if strings.Contains(out, "# qh") {
		t.Errorf("expected no qh markers, got %q", out)
	}
}

func TestAddAndSaveRemoveAndSave(t *testing.T) {
	reset()
	p := writeHosts(t, "127.0.0.1 localhost\n")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("example.com", "1.2.3.4", p, nil); err != nil {
		t.Fatal(err)
	}
	out := readHosts(t, p)
	if !strings.Contains(out, "1.2.3.4 example.com") {
		t.Fatalf("entry not added: %q", out)
	}
	if !strings.Contains(out, "127.0.0.1 localhost") {
		t.Fatalf("original entry dropped: %q", out)
	}

	// reload and remove
	reset()
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := RemoveAndSave("example.com", p); err != nil {
		t.Fatal(err)
	}
	out = readHosts(t, p)
	if strings.Contains(out, "example.com") {
		t.Errorf("entry not removed: %q", out)
	}
	if strings.Contains(out, "# qh") {
		t.Errorf("qh markers should be gone after last entry removed: %q", out)
	}
}

func TestAddOverwritesExistingDomain(t *testing.T) {
	reset()
	p := writeHosts(t, "")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("example.com", "1.2.3.4", p, nil); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("example.com", "5.6.7.8", p, nil); err != nil {
		t.Fatal(err)
	}
	if got := Hosts["1.2.3.4"]; len(got) != 0 {
		t.Errorf("old IP still mapped: %v", got)
	}
	if got := Hosts["5.6.7.8"]; !equalSlices(got, []string{"example.com"}) {
		t.Errorf("new IP mapping = %v, want [example.com]", got)
	}
}

func TestLookupAndReverseLookup(t *testing.T) {
	reset()
	Hosts = map[string][]string{
		"1.2.3.4": {"example.com", "alias.example.com"},
		"5.6.7.8": {"other.com"},
	}
	got, err := Lookup("EXAMPLE.com")
	if err != nil {
		t.Fatal(err)
	}
	if !equalSlices(got, []string{"1.2.3.4"}) {
		t.Errorf("Lookup = %v, want [1.2.3.4]", got)
	}
	rev, err := ReverseLookup("5.6.7.8")
	if err != nil {
		t.Fatal(err)
	}
	if !equalSlices(rev, []string{"other.com"}) {
		t.Errorf("ReverseLookup = %v, want [other.com]", rev)
	}
}

func TestIpForDomainLiteralAndLocalhost(t *testing.T) {
	got, err := ipForDomain("1.2.3.4", nil)
	if err != nil || got != "1.2.3.4" {
		t.Errorf("literal: got (%q,%v), want 1.2.3.4,nil", got, err)
	}
	got, err = ipForDomain("localhost", nil)
	if err != nil || got != "127.0.0.1" {
		t.Errorf("localhost: got (%q,%v), want 127.0.0.1,nil", got, err)
	}
}

func TestIpForDomainCustomResolver(t *testing.T) {
	addr := startMockDNS(t, net.IPv4(9, 9, 9, 9))
	got, err := ipForDomain("anything.example", []string{addr})
	if err != nil {
		t.Fatal(err)
	}
	if got != "9.9.9.9" {
		t.Errorf("got %q, want 9.9.9.9", got)
	}
}

func TestIpForDomainCustomResolverIsActuallyUsed(t *testing.T) {
	// Point at a closed UDP socket — if the code falls back to the system
	// resolver, this test would silently pass. With the custom resolver
	// path, the lookup must fail.
	addr := closedUDP(t)
	_, err := ipForDomain("definitely-does-not-exist.example", []string{addr})
	if err == nil {
		t.Fatal("expected error from unreachable resolver, got nil")
	}
}

func TestIpForDomainResolverFallover(t *testing.T) {
	// First server is a black hole, second answers — verify fallover.
	bad := closedUDP(t)
	good := startMockDNS(t, net.IPv4(7, 7, 7, 7))
	got, err := ipForDomain("foo.example", []string{bad, good})
	if err != nil {
		t.Fatal(err)
	}
	if got != "7.7.7.7" {
		t.Errorf("got %q, want 7.7.7.7", got)
	}
}

func TestAddWithCustomResolverResolves(t *testing.T) {
	reset()
	addr := startMockDNS(t, net.IPv4(8, 8, 8, 8))
	p := writeHosts(t, "")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("foo.example", "foo.example", p, []string{addr}); err != nil {
		t.Fatal(err)
	}
	if got := Hosts["8.8.8.8"]; !equalSlices(got, []string{"foo.example"}) {
		t.Errorf("Hosts[8.8.8.8] = %v, want [foo.example]", got)
	}
	out := readHosts(t, p)
	if !strings.Contains(out, "8.8.8.8 foo.example") {
		t.Errorf("hosts file missing resolved entry: %q", out)
	}
}

func TestResolveResolverSpec(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
		err  bool
	}{
		{"empty", "", nil, false},
		{"named", "cloudflare", []string{"1.1.1.1:53"}, false},
		{"named case insensitive", "Google", []string{"8.8.8.8:53"}, false},
		{"alias cf", "cf", []string{"1.1.1.1:53"}, false},
		{"alias q9", "q9", []string{"9.9.9.9:53"}, false},
		{"alias od case insensitive", "OD", []string{"208.67.222.222:53"}, false},
		{"mixed names and aliases", "cf,go,q9", []string{"1.1.1.1:53", "8.8.8.8:53", "9.9.9.9:53"}, false},
		{"bare ip", "1.2.3.4", []string{"1.2.3.4:53"}, false},
		{"ip with port", "1.2.3.4:5353", []string{"1.2.3.4:5353"}, false},
		{"comma list", "cloudflare,8.8.4.4", []string{"1.1.1.1:53", "8.8.4.4:53"}, false},
		{"whitespace", " quad9 , 9.9.9.10 ", []string{"9.9.9.9:53", "9.9.9.10:53"}, false},
		{"only commas", ",,", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ResolveResolverSpec(c.in)
			if c.err {
				if err == nil {
					t.Fatalf("expected error, got %v", got)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !equalSlices(got, c.want) {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}
}

func TestFlush(t *testing.T) {
	reset()
	p := writeHosts(t, "127.0.0.1 localhost\n")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("a.example", "1.1.1.1", p, nil); err != nil {
		t.Fatal(err)
	}
	if err := AddAndSave("b.example", "2.2.2.2", p, nil); err != nil {
		t.Fatal(err)
	}
	if err := Flush(p); err != nil {
		t.Fatal(err)
	}
	out := readHosts(t, p)
	if strings.Contains(out, "a.example") || strings.Contains(out, "b.example") {
		t.Errorf("flush left entries behind: %q", out)
	}
	if strings.Contains(out, "# qh") {
		t.Errorf("flush left qh markers: %q", out)
	}
	if !strings.Contains(out, "127.0.0.1 localhost") {
		t.Errorf("flush dropped unrelated entries: %q", out)
	}
}

func TestList(t *testing.T) {
	reset()
	if got := List(); got != "" {
		t.Errorf("empty list = %q, want \"\"", got)
	}
	Hosts = map[string][]string{
		"2.2.2.2": {"b.example"},
		"1.1.1.1": {"a.example", "alias.a"},
	}
	got := List()
	want := "1.1.1.1 a.example alias.a\n2.2.2.2 b.example\n"
	if got != want {
		t.Errorf("List() =\n%q\nwant\n%q", got, want)
	}
}

func TestAddTempTTL(t *testing.T) {
	reset()
	p := writeHosts(t, "")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	entries := []Entry{
		{Domain: "a.example", Target: "1.1.1.1"},
		{Domain: "b.example", Target: "2.2.2.2"},
	}
	start := time.Now()
	if err := AddTemp(entries, p, nil, 100*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed < 80*time.Millisecond {
		t.Errorf("returned too early: %v", elapsed)
	}
	out := readHosts(t, p)
	if strings.Contains(out, "a.example") || strings.Contains(out, "b.example") {
		t.Errorf("ttl did not clean up entries: %q", out)
	}
}

func TestAddTempCleansUpOnAddFailure(t *testing.T) {
	reset()
	p := writeHosts(t, "")
	if err := Load(p); err != nil {
		t.Fatal(err)
	}
	// Second entry has an unreachable resolver and should fail; first
	// entry should be rolled back.
	bad := closedUDP(t)
	entries := []Entry{
		{Domain: "good.example", Target: "1.1.1.1"},
		{Domain: "bad.example", Target: "bad.example"},
	}
	if err := AddTemp(entries, p, []string{bad}, 0); err == nil {
		t.Fatal("expected error from unreachable resolver")
	}
	out := readHosts(t, p)
	if strings.Contains(out, "good.example") {
		t.Errorf("rollback failed: %q", out)
	}
}

func TestAvailableAndFormatResolverNames(t *testing.T) {
	names := AvailableResolverNames()
	if len(names) != len(NamedResolvers) {
		t.Errorf("AvailableResolverNames: got %d, want %d", len(names), len(NamedResolvers))
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Errorf("AvailableResolverNames not sorted: %v", names)
			break
		}
	}
	formatted := FormatResolverNames()
	for n, ip := range NamedResolvers {
		if !strings.Contains(formatted, n) {
			t.Errorf("formatted missing name %q: %s", n, formatted)
		}
		if !strings.Contains(formatted, ip) {
			t.Errorf("formatted missing ip %q: %s", ip, formatted)
		}
	}
	for alias := range NamedResolverAliases {
		if !strings.Contains(formatted, alias) {
			t.Errorf("formatted missing alias %q: %s", alias, formatted)
		}
	}
}

func TestLoadMissingFile(t *testing.T) {
	if err := Load(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Fatal("expected error loading missing file")
	}
}

func TestSavePermissionDenied(t *testing.T) {
	reset()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })
	Hosts = map[string][]string{"1.2.3.4": {"x.example"}}
	err := Save(filepath.Join(dir, "hosts"))
	if err == nil {
		t.Fatal("expected error writing to read-only dir")
	}
}

func TestFirstIPv4NoA(t *testing.T) {
	if _, err := firstIPv4(nil); err == nil {
		t.Error("nil: expected error")
	}
	v6 := net.ParseIP("::1")
	if _, err := firstIPv4([]net.IP{v6}); err == nil {
		t.Error("v6-only: expected error")
	}
}

func TestIpForDomainNoARecord(t *testing.T) {
	// Mock that responds with ANCOUNT=0 — exercise the lastErr path.
	addr := startEmptyDNS(t)
	if _, err := ipForDomain("nope.example", []string{addr}); err == nil {
		t.Fatal("expected error when resolver returns no answers")
	}
}

func TestAddResolutionFailure(t *testing.T) {
	reset()
	bad := closedUDP(t)
	if err := Add("foo.example", "foo.example", []string{bad}); err == nil {
		t.Fatal("expected error from unreachable resolver")
	}
	if len(Hosts) != 0 {
		t.Errorf("Hosts should be empty after failed Add: %v", Hosts)
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// startMockDNS spins up a tiny UDP DNS responder bound to a random localhost
// port. It answers every A query with the supplied IPv4 address. Returns the
// "host:port" address suitable for passing as a -resolver value.
func startMockDNS(t *testing.T, ip net.IP) string {
	t.Helper()
	ip4 := ip.To4()
	if ip4 == nil {
		t.Fatalf("startMockDNS requires IPv4, got %v", ip)
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		conn.Close()
		<-done
	})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					select {
					case <-done:
						return
					default:
						continue
					}
				}
				return
			}
			if n < 12 {
				continue
			}
			// Walk the question section to find where it ends, so we can
			// drop any additional records (e.g. EDNS0 OPT) the client sent.
			qEnd := 12
			for qEnd < n && buf[qEnd] != 0 {
				if buf[qEnd]&0xC0 == 0xC0 {
					qEnd += 2
					goto past
				}
				qEnd += int(buf[qEnd]) + 1
			}
			if qEnd < n {
				qEnd++ // null terminator
			}
		past:
			qEnd += 4 // QTYPE + QCLASS
			if qEnd > n {
				continue
			}
			resp := make([]byte, qEnd, qEnd+16)
			copy(resp, buf[:qEnd])
			resp[2] |= 0x80                          // QR = response
			resp[3] |= 0x80                          // RA
			resp[6], resp[7] = 0x00, 0x01            // ANCOUNT = 1
			resp[8], resp[9] = 0x00, 0x00            // NSCOUNT
			resp[10], resp[11] = 0x00, 0x00          // ARCOUNT
			resp = append(resp,
				0xC0, 0x0C, // NAME: pointer to question
				0x00, 0x01, // TYPE A
				0x00, 0x01, // CLASS IN
				0x00, 0x00, 0x00, 0x3C, // TTL 60
				0x00, 0x04, // RDLENGTH
				ip4[0], ip4[1], ip4[2], ip4[3],
			)
			conn.WriteToUDP(resp, addr)
		}
	}()
	return conn.LocalAddr().String()
}

// startEmptyDNS spins up a UDP DNS responder that always replies with
// ANCOUNT=0 (no answer records).
func startEmptyDNS(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	t.Cleanup(func() {
		conn.Close()
		<-done
	})
	go func() {
		defer close(done)
		buf := make([]byte, 512)
		for {
			conn.SetReadDeadline(time.Now().Add(2 * time.Second))
			n, addr, err := conn.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					select {
					case <-done:
						return
					default:
						continue
					}
				}
				return
			}
			if n < 12 {
				continue
			}
			qEnd := 12
			for qEnd < n && buf[qEnd] != 0 {
				if buf[qEnd]&0xC0 == 0xC0 {
					qEnd += 2
					goto past
				}
				qEnd += int(buf[qEnd]) + 1
			}
			if qEnd < n {
				qEnd++
			}
		past:
			qEnd += 4
			if qEnd > n {
				continue
			}
			resp := make([]byte, qEnd)
			copy(resp, buf[:qEnd])
			resp[2] |= 0x80
			resp[3] |= 0x80
			resp[6], resp[7] = 0x00, 0x00 // ANCOUNT = 0
			resp[8], resp[9] = 0x00, 0x00
			resp[10], resp[11] = 0x00, 0x00
			conn.WriteToUDP(resp, addr)
		}
	}()
	return conn.LocalAddr().String()
}

// closedUDP returns a host:port that nothing is listening on.
func closedUDP(t *testing.T) string {
	t.Helper()
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	addr := conn.LocalAddr().String()
	conn.Close()
	return addr
}
