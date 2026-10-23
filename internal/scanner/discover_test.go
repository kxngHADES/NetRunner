package scanner

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseSubnets(t *testing.T) {
	subnets, hosts, err := ParseSubnets("10.0.0.0/30")
	if err != nil || !reflect.DeepEqual(subnets, []string{"10.0.0.0/30"}) ||
		!reflect.DeepEqual(hosts, []string{"10.0.0.1", "10.0.0.2"}) {
		t.Errorf("/30: %v %v %v", subnets, hosts, err)
	}
	// Host bits are masked, /31 and /32 keep every address, overlaps are merged.
	subnets, hosts, err = ParseSubnets("10.0.0.5/31, 10.0.0.4/32,10.0.0.9")
	if err != nil || !reflect.DeepEqual(subnets, []string{"10.0.0.4/31", "10.0.0.4/32", "10.0.0.9/32"}) ||
		!reflect.DeepEqual(hosts, []string{"10.0.0.4", "10.0.0.5", "10.0.0.9"}) {
		t.Errorf("mixed: %v %v %v", subnets, hosts, err)
	}
	if _, hosts, err := ParseSubnets("192.168.1.0/24"); err != nil || len(hosts) != 254 {
		t.Errorf("/24: %d hosts, %v", len(hosts), err)
	}
	for _, in := range []string{"", "nope", "10.0.0.0/33", "10.0.0.0/8", "::1/128", "10.0.0.0/24,", "host.example/24",
		"10.0.0.0/16,10.1.0.0/16"} {
		if _, _, err := ParseSubnets(in); err == nil {
			t.Errorf("ParseSubnets(%q): want error", in)
		}
	}
}

func TestDiscoverSkipsAnsweredHosts(t *testing.T) {
	var mu sync.Mutex
	probed := map[string]int{}
	check := func(ctx context.Context, host string, port uint16, _ time.Duration) PortResult {
		mu.Lock()
		probed[host]++
		mu.Unlock()
		r := PortResult{Host: host, Port: port, Protocol: "tcp", State: StateFiltered}
		switch {
		case host == "10.0.0.1" && port == 1:
			r.State = StateOpen
		case host == "10.0.0.3" && port == 3:
			r.State = StateClosed
		}
		return r
	}
	cfg := Config{Targets: []string{"10.0.0.3", "10.0.0.1", "10.0.0.2"}, Ports: []uint16{1, 2, 3}, Workers: 1, Timeout: time.Second}
	var last DiscoverProgress
	found := 0
	out := Discover(context.Background(), cfg, check, func(p DiscoverProgress) {
		last = p
		if p.Found != nil {
			found++
		}
	})
	if out.Incomplete || out.HostsDone != 3 || len(out.Alive) != 2 {
		t.Fatalf("outcome: %+v", out)
	}
	if out.Alive[0].IP != "10.0.0.1" || out.Alive[0].State != StateOpen || out.Alive[1].IP != "10.0.0.3" || out.Alive[1].Port != 3 {
		t.Errorf("alive: %+v", out.Alive)
	}
	// With one worker, 10.0.0.1 answers on its first port so ports 2 and 3 are skipped.
	if probed["10.0.0.1"] != 1 || probed["10.0.0.2"] != 3 || out.Probes != 7 {
		t.Errorf("probed=%v probes=%d", probed, out.Probes)
	}
	if found != 2 || last.HostsDone != 3 || last.Alive != 2 {
		t.Errorf("progress: found=%d last=%+v", found, last)
	}
}

func TestDiscoverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	check := func(ctx context.Context, host string, port uint16, _ time.Duration) PortResult {
		if calls.Add(1) == 5 {
			cancel()
		}
		if ctx.Err() != nil {
			return PortResult{Host: host, Port: port, State: stateCancelled}
		}
		return PortResult{Host: host, Port: port, State: StateFiltered}
	}
	_, hosts, _ := ParseSubnets("10.0.0.0/24")
	cfg := Config{Targets: hosts, Ports: []uint16{1, 2}, Workers: 4, Timeout: time.Second}
	out := Discover(ctx, cfg, check, nil)
	if !out.Incomplete || out.HostsDone >= len(hosts) {
		t.Errorf("expected incomplete outcome: %+v", out)
	}
	rep := BuildDiscoveryReport("test", []string{"10.0.0.0/24"}, cfg, out, time.Now(), time.Now())
	if rep.Status != StatusIncomplete || rep.Hosts == nil || rep.Summary.HostsPlanned != 254 {
		t.Errorf("report: %+v", rep)
	}
}
