package scanner

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// MaxDiscoverHosts caps how many addresses one discovery run may probe (a /16).
const MaxDiscoverHosts = 1 << 16

// DefaultDiscoverProfile is the probe port set used when discover --ports is not given.
const DefaultDiscoverProfile = "discover"

// ParseSubnets parses a comma-separated list of IPv4 CIDR blocks (or single
// addresses) and expands them into host addresses. Network and broadcast
// addresses are skipped for prefixes shorter than /31. Duplicates are removed
// and the result is sorted.
func ParseSubnets(spec string) (subnets []string, hosts []string, err error) {
	if strings.TrimSpace(spec) == "" {
		return nil, nil, errors.New("no subnet specified")
	}
	seen := make(map[netip.Addr]struct{})
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return nil, nil, fmt.Errorf("invalid subnet list %q: empty entry", spec)
		}
		var pfx netip.Prefix
		if strings.Contains(tok, "/") {
			if pfx, err = netip.ParsePrefix(tok); err != nil {
				return nil, nil, fmt.Errorf("invalid subnet %q: expected CIDR such as 192.168.1.0/24", tok)
			}
		} else {
			addr, perr := netip.ParseAddr(tok)
			if perr != nil {
				return nil, nil, fmt.Errorf("invalid subnet %q: expected CIDR such as 192.168.1.0/24", tok)
			}
			pfx = netip.PrefixFrom(addr, addr.BitLen())
		}
		if !pfx.Addr().Is4() {
			return nil, nil, fmt.Errorf("invalid subnet %q: only IPv4 is supported", tok)
		}
		if pfx.Bits() < 16 {
			return nil, nil, fmt.Errorf("invalid subnet %q: prefix must be /16 or longer (at most 65536 addresses)", tok)
		}
		pfx = pfx.Masked()
		subnets = append(subnets, pfx.String())

		size := 1 << (32 - pfx.Bits())
		addr := pfx.Addr()
		for i := 0; i < size; i++ {
			edge := i == 0 || i == size-1
			if !(edge && pfx.Bits() < 31) {
				seen[addr] = struct{}{}
			}
			addr = addr.Next()
		}
		if len(seen) > MaxDiscoverHosts {
			return nil, nil, fmt.Errorf("too many hosts: at most %d per run", MaxDiscoverHosts)
		}
	}
	addrs := make([]netip.Addr, 0, len(seen))
	for a := range seen {
		addrs = append(addrs, a)
	}
	sort.Slice(addrs, func(i, j int) bool { return addrs[i].Less(addrs[j]) })
	hosts = make([]string, len(addrs))
	for i, a := range addrs {
		hosts[i] = a.String()
	}
	return subnets, hosts, nil
}

// LiveHost is a host that answered at least one probe. Evidence is the first
// probe that proved it alive: an accepted or a refused connection.
type LiveHost struct {
	IP        string  `json:"ip"`
	Port      uint16  `json:"port"`
	State     string  `json:"state"`
	Service   string  `json:"service,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
}

// DiscoverProgress is a snapshot passed to the discovery progress callback.
type DiscoverProgress struct {
	HostsDone  int
	HostsTotal int
	Alive      int
	Active     int
	Probes     int
	Found      *LiveHost // set when this update discovered a new live host
}

// DiscoverOutcome is the raw result of a discovery run.
type DiscoverOutcome struct {
	Alive      []LiveHost
	HostsTotal int
	HostsDone  int
	Probes     int
	Incomplete bool
}

type probeJob struct {
	idx  int
	port uint16
}

type probeEvent struct {
	idx     int
	skipped bool
	res     PortResult
}

func answered(state string) bool { return state == StateOpen || state == StateClosed }

// Discover probes every target on cfg.Ports with a bounded worker pool and
// reports the hosts that answer. Once a host has answered, its remaining
// probes are skipped. onProgress (optional) is called from the calling
// goroutine after every finished or skipped probe.
func Discover(ctx context.Context, cfg Config, check Checker, onProgress func(DiscoverProgress)) DiscoverOutcome {
	n := len(cfg.Targets)
	alive := make([]atomic.Bool, n)
	jobs := make(chan probeJob, cfg.Workers)
	events := make(chan probeEvent, cfg.Workers)

	var active atomic.Int32
	var wg sync.WaitGroup

	go func() {
		defer close(jobs)
		for i := range cfg.Targets {
			for _, p := range cfg.Ports {
				select {
				case jobs <- probeJob{i, p}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					continue
				}
				if alive[j.idx].Load() {
					events <- probeEvent{idx: j.idx, skipped: true}
					continue
				}
				active.Add(1)
				r := check(ctx, cfg.Targets[j.idx], j.port, cfg.Timeout)
				active.Add(-1)
				if r.State == stateCancelled {
					continue
				}
				if answered(r.State) {
					alive[j.idx].Store(true)
				}
				events <- probeEvent{idx: j.idx, res: r}
			}
		}()
	}

	go func() {
		wg.Wait()
		close(events)
	}()

	out := DiscoverOutcome{HostsTotal: n}
	remaining := make([]int, n)
	for i := range remaining {
		remaining[i] = len(cfg.Ports)
	}
	found := make([]bool, n)
	for ev := range events {
		var newHost *LiveHost
		if !ev.skipped {
			out.Probes++
			if answered(ev.res.State) && !found[ev.idx] {
				found[ev.idx] = true
				h := LiveHost{
					IP:        ev.res.Host,
					Port:      ev.res.Port,
					State:     ev.res.State,
					Service:   ev.res.Service,
					LatencyMS: ev.res.LatencyMS,
				}
				out.Alive = append(out.Alive, h)
				newHost = &h
			}
		}
		remaining[ev.idx]--
		if remaining[ev.idx] == 0 {
			out.HostsDone++
		}
		if onProgress != nil {
			onProgress(DiscoverProgress{
				HostsDone:  out.HostsDone,
				HostsTotal: n,
				Alive:      len(out.Alive),
				Active:     int(active.Load()),
				Probes:     out.Probes,
				Found:      newHost,
			})
		}
	}
	// A host proven alive is a definite result even if its skipped probes
	// never ran, so only unfinished hosts that never answered make it partial.
	for i := range remaining {
		if remaining[i] > 0 && found[i] {
			out.HostsDone++
		}
	}
	out.Incomplete = out.HostsDone < n

	sort.Slice(out.Alive, func(i, j int) bool {
		a, _ := netip.ParseAddr(out.Alive[i].IP)
		b, _ := netip.ParseAddr(out.Alive[j].IP)
		return a.Less(b)
	})
	return out
}

// DiscoverySettings records the parameters of a discovery run.
type DiscoverySettings struct {
	Workers   int      `json:"workers"`
	TimeoutMS int64    `json:"timeout_ms"`
	Ports     []uint16 `json:"probe_ports"`
	Protocol  string   `json:"protocol"`
}

// DiscoverySummary holds aggregate counts for a discovery run.
type DiscoverySummary struct {
	HostsPlanned int `json:"hosts_planned"`
	HostsProbed  int `json:"hosts_probed"`
	HostsAlive   int `json:"hosts_alive"`
	ProbesSent   int `json:"probes_sent"`
}

// DiscoveryReport is the versioned top-level discovery report.
type DiscoveryReport struct {
	SchemaVersion int               `json:"schema_version"`
	Kind          string            `json:"kind"`
	ScanID        string            `json:"scan_id"`
	Tool          string            `json:"tool"`
	Status        string            `json:"status"`
	StartedAt     time.Time         `json:"started_at"`
	CompletedAt   time.Time         `json:"completed_at"`
	DurationMS    int64             `json:"duration_ms"`
	Subnets       []string          `json:"subnets"`
	Settings      DiscoverySettings `json:"settings"`
	Summary       DiscoverySummary  `json:"summary"`
	Hosts         []LiveHost        `json:"hosts"`
}

// BuildDiscoveryReport assembles a report from a finished (or cancelled) discovery run.
func BuildDiscoveryReport(tool string, subnets []string, cfg Config, out DiscoverOutcome, start, end time.Time) DiscoveryReport {
	rep := DiscoveryReport{
		SchemaVersion: SchemaVersion,
		Kind:          "discovery",
		ScanID:        newID(),
		Tool:          tool,
		Status:        StatusComplete,
		StartedAt:     start.UTC(),
		CompletedAt:   end.UTC(),
		DurationMS:    end.Sub(start).Milliseconds(),
		Subnets:       subnets,
		Settings: DiscoverySettings{
			Workers:   cfg.Workers,
			TimeoutMS: cfg.Timeout.Milliseconds(),
			Ports:     cfg.Ports,
			Protocol:  "tcp",
		},
		Summary: DiscoverySummary{
			HostsPlanned: out.HostsTotal,
			HostsProbed:  out.HostsDone,
			HostsAlive:   len(out.Alive),
			ProbesSent:   out.Probes,
		},
		Hosts: out.Alive,
	}
	if rep.Hosts == nil {
		rep.Hosts = []LiveHost{}
	}
	if out.Incomplete {
		rep.Status = StatusIncomplete
	}
	return rep
}
