package scanner

import (
	"context"
	"net"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParsePorts(t *testing.T) {
	good := []struct {
		in   string
		want []uint16
	}{
		{"80", []uint16{80}},
		{"22,80,443", []uint16{22, 80, 443}},
		{"80,22,80,22-24", []uint16{22, 23, 24, 80}},
		{"8000-8002", []uint16{8000, 8001, 8002}},
		{" 22 , 80 ", []uint16{22, 80}},
		{"65535", []uint16{65535}},
	}
	for _, c := range good {
		got, err := ParsePorts(c.in)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("ParsePorts(%q) = %v, %v; want %v", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{"", " ", "0", "65536", "-1", "abc", "80,", ",80", "10-5", "1-70000", "1-2-3"} {
		if got, err := ParsePorts(in); err == nil {
			t.Errorf("ParsePorts(%q) = %v; want error", in, got)
		}
	}
}

func TestParsePortsProfiles(t *testing.T) {
	got, err := ParsePorts("web,22")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || got[0] != 22 {
		t.Errorf("unexpected profile expansion: %v", got)
	}
}

func TestParseTargets(t *testing.T) {
	got, err := ParseTargets("127.0.0.1, localhost,127.0.0.1")
	if err != nil || !reflect.DeepEqual(got, []string{"127.0.0.1", "localhost"}) {
		t.Errorf("got %v, %v", got, err)
	}
	for _, in := range []string{"", "192.168.1.0/24", "::1", "bad host", "-bad.example", "1.2.3.4,,"} {
		if got, err := ParseTargets(in); err == nil {
			t.Errorf("ParseTargets(%q) = %v; want error", in, got)
		}
	}
}

func TestConfigValidate(t *testing.T) {
	ok := Config{Targets: []string{"127.0.0.1"}, Ports: []uint16{80}, Workers: 10, Timeout: time.Second}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := []func(*Config){
		func(c *Config) { c.Targets = nil },
		func(c *Config) { c.Ports = nil },
		func(c *Config) { c.Workers = 0 },
		func(c *Config) { c.Workers = MaxWorkers + 1 },
		func(c *Config) { c.Timeout = 0 },
		func(c *Config) { c.Timeout = time.Hour },
	}
	for i, mut := range bad {
		c := ok
		mut(&c)
		if c.Validate() == nil {
			t.Errorf("case %d: expected validation error", i)
		}
	}
}

func listenerPort(t *testing.T, l net.Listener) uint16 {
	t.Helper()
	return uint16(l.Addr().(*net.TCPAddr).Port)
}

func TestCheckTCPOpenAndClosed(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	openPort := listenerPort(t, l)

	r := CheckTCP(context.Background(), "127.0.0.1", openPort, time.Second)
	if r.State != StateOpen || r.Error != "" {
		t.Errorf("open port: %+v", r)
	}

	l.Close() // port is now unused: connection should be refused
	r = CheckTCP(context.Background(), "127.0.0.1", openPort, time.Second)
	if r.State != StateClosed {
		t.Errorf("closed port: %+v", r)
	}
}

func TestCheckTCPCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// 192.0.2.1 is TEST-NET-1 (never routable); the context is already done.
	r := CheckTCP(ctx, "192.0.2.1", 80, 5*time.Second)
	if r.State != stateCancelled {
		t.Errorf("got %+v", r)
	}
}

func TestScanWorkerLimitAndCounts(t *testing.T) {
	const workers = 5
	var cur, peak atomic.Int32
	check := func(ctx context.Context, host string, port uint16, _ time.Duration) PortResult {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		return PortResult{Host: host, Port: port, Protocol: "tcp", State: StateOpen}
	}
	ports := make([]uint16, 100)
	for i := range ports {
		ports[i] = uint16(i + 1)
	}
	cfg := Config{Targets: []string{"a", "b"}, Ports: ports, Workers: workers, Timeout: time.Second}
	out := Scan(context.Background(), cfg, check, nil)
	if len(out.Results) != 200 || out.Incomplete || out.Total != 200 {
		t.Errorf("results=%d incomplete=%v total=%d", len(out.Results), out.Incomplete, out.Total)
	}
	if peak.Load() > workers {
		t.Errorf("peak concurrency %d exceeded %d workers", peak.Load(), workers)
	}
	if out.Results[0].Host != "a" || out.Results[199].Host != "b" {
		t.Error("results not ordered by target")
	}
}

func TestScanCancellationNoLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	var started sync.Once
	ctx, cancel := context.WithCancel(context.Background())
	check := func(ctx context.Context, host string, port uint16, _ time.Duration) PortResult {
		started.Do(cancel)
		<-ctx.Done()
		return PortResult{Host: host, Port: port, State: stateCancelled}
	}
	ports := make([]uint16, 1000)
	for i := range ports {
		ports[i] = uint16(i + 1)
	}
	cfg := Config{Targets: []string{"x"}, Ports: ports, Workers: 20, Timeout: time.Minute}

	done := make(chan Outcome)
	go func() { done <- Scan(ctx, cfg, check, nil) }()
	select {
	case out := <-done:
		if !out.Incomplete {
			t.Error("expected incomplete outcome")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Scan did not return after cancellation")
	}

	deadline := time.Now().Add(2 * time.Second)
	for runtime.NumGoroutine() > before+1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before+1 {
		t.Errorf("goroutines before=%d after=%d", before, n)
	}
}

func TestBuildReport(t *testing.T) {
	cfg := Config{Targets: []string{"10.0.0.1"}, Ports: []uint16{80, 81, 82}, Workers: 1, Timeout: time.Second}
	out := Outcome{Total: 3, Results: []PortResult{
		{Host: "10.0.0.1", Port: 82, State: StateFiltered, Error: "timeout"},
		{Host: "10.0.0.1", Port: 80, State: StateOpen},
		{Host: "10.0.0.1", Port: 81, State: StateClosed},
	}}
	now := time.Now()
	rep := BuildReport("test", []HostInfo{{IP: "10.0.0.1", Hostname: "box"}}, cfg, out, now, now.Add(time.Second))
	if rep.SchemaVersion != SchemaVersion || rep.Status != StatusComplete {
		t.Errorf("header: %+v", rep)
	}
	if s := rep.Summary; s.Open != 1 || s.Closed != 1 || s.FilteredOrUnk != 1 || s.PortsChecked != 3 {
		t.Errorf("summary: %+v", s)
	}
	h := rep.Hosts[0]
	if !h.Reachable || h.Ports[0].Port != 80 || rep.Targets[0] != "box" {
		t.Errorf("host: %+v", h)
	}
	out.Incomplete = true
	if r := BuildReport("test", nil, cfg, out, now, now); r.Status != StatusIncomplete {
		t.Error("expected incomplete status")
	}
}
