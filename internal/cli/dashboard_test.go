package cli

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/kxngHADES/NetRunner/internal/scanner"
)

func TestDashboardNotATerminal(t *testing.T) {
	if d := newDashboard(&bytes.Buffer{}, "scan", "", "ports", 1, 1, false); d != nil {
		t.Error("dashboard enabled on a non-terminal writer")
	}
}

func checkFrame(t *testing.T, lines []string, want ...string) {
	t.Helper()
	for _, l := range lines {
		if n := utf8.RuneCountInString(l); n > dashWidth {
			t.Errorf("line is %d columns wide: %q", n, l)
		}
	}
	all := strings.Join(lines, "\n")
	for _, w := range want {
		if !strings.Contains(all, w) {
			t.Errorf("frame missing %q:\n%s", w, all)
		}
	}
}

func TestDashboardScanFrame(t *testing.T) {
	d := &dashboard{kind: "scan", scope: "target x", unit: "ports", workers: 10, total: 4,
		start: time.Now(), byIP: map[string]*dashHost{}}
	d.addHosts([]scanner.HostInfo{
		{IP: "10.0.0.1", Hostname: "a-very-long-hostname-that-needs-truncating.example"},
		{IP: "10.0.0.2"},
	}, 2)
	d.onScan(scanner.Progress{Done: 1, Total: 4, Active: 3, Result: scanner.PortResult{Host: "10.0.0.1", Port: 22, State: scanner.StateOpen, Service: "ssh", LatencyMS: 1.5}})
	d.onScan(scanner.Progress{Done: 2, Total: 4, Active: 2, Result: scanner.PortResult{Host: "10.0.0.1", Port: 80, State: scanner.StateClosed}})
	d.onScan(scanner.Progress{Done: 3, Total: 4, Active: 1, Result: scanner.PortResult{Host: "10.0.0.2", Port: 22, State: scanner.StateFiltered}})

	checkFrame(t, d.frame(""), "RUNNING", "3/4 ports", "1/10 busy", "open 1", "closed 1", "filtered/unknown 1",
		"done", "scanning", "22/tcp", "ssh", "1.5ms", "Open ports (1)")
	checkFrame(t, d.frame("INTERRUPTED"), "INTERRUPTED")
}

func TestDashboardDiscoverFrame(t *testing.T) {
	d := &dashboard{kind: "discover", scope: "subnet 10.0.0.0/24", unit: "hosts", workers: 100, total: 254,
		start: time.Now(), byIP: map[string]*dashHost{}}
	for i := 0; i < dashMaxFeed+3; i++ {
		d.onDiscover(scanner.DiscoverProgress{HostsDone: i + 1, HostsTotal: 254, Alive: i + 1, Probes: 40,
			Found: &scanner.LiveHost{IP: "10.0.0.1", Port: 445, State: scanner.StateOpen, Service: "smb"}})
	}
	lines := d.frame("COMPLETE")
	checkFrame(t, lines, "COMPLETE", "8/254 hosts", "alive 8", "probes sent 40", "Live hosts (8)", "3 earlier", "445/tcp")
	if len(lines) > 24 {
		t.Errorf("frame has %d lines", len(lines))
	}
}

func TestDashboardDrawRewinds(t *testing.T) {
	var buf bytes.Buffer
	d := &dashboard{w: &buf, kind: "discover", unit: "hosts", workers: 1, total: 1, start: time.Now()}
	d.draw("")
	n := d.lines
	buf.Reset()
	d.draw("")
	if !strings.HasPrefix(buf.String(), "\x1b["+strconv.Itoa(n)+"F") {
		t.Errorf("second frame does not move back %d lines: %q", n, buf.String()[:10])
	}
}
