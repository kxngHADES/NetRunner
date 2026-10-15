package scanner

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"time"
)

// SchemaVersion is the version of the JSON report format (see docs/report-schema.md).
const SchemaVersion = 1

// Port states.
const (
	StateOpen     = "open"
	StateClosed   = "closed"
	StateFiltered = "filtered_or_unknown"

	// stateCancelled marks an attempt aborted by cancellation; never reported.
	stateCancelled = "cancelled"
)

// Report statuses.
const (
	StatusComplete   = "complete"
	StatusIncomplete = "incomplete"
)

// PortResult is the outcome of one TCP connection attempt.
type PortResult struct {
	Host      string  `json:"host"`
	Port      uint16  `json:"port"`
	Protocol  string  `json:"protocol"`
	State     string  `json:"state"`
	Service   string  `json:"service,omitempty"`
	LatencyMS float64 `json:"latency_ms,omitempty"`
	Error     string  `json:"error,omitempty"`
}

// HostResult groups the port results for one host.
type HostResult struct {
	IP        string       `json:"ip"`
	Hostname  string       `json:"hostname,omitempty"`
	Reachable bool         `json:"reachable"`
	Ports     []PortResult `json:"ports"`
}

// Settings records the scan parameters used, so a report states its own scope.
type Settings struct {
	Workers   int    `json:"workers"`
	TimeoutMS int64  `json:"timeout_ms"`
	Ports     int    `json:"ports_per_host"`
	Protocol  string `json:"protocol"`
}

// Summary holds aggregate counts.
type Summary struct {
	HostsScanned  int `json:"hosts_scanned"`
	PortsPlanned  int `json:"ports_planned"`
	PortsChecked  int `json:"ports_checked"`
	Open          int `json:"open"`
	Closed        int `json:"closed"`
	FilteredOrUnk int `json:"filtered_or_unknown"`
}

// ScanReport is the versioned top-level report.
type ScanReport struct {
	SchemaVersion int          `json:"schema_version"`
	ScanID        string       `json:"scan_id"`
	Tool          string       `json:"tool"`
	Status        string       `json:"status"`
	StartedAt     time.Time    `json:"started_at"`
	CompletedAt   time.Time    `json:"completed_at"`
	DurationMS    int64        `json:"duration_ms"`
	Targets       []string     `json:"targets"`
	Settings      Settings     `json:"settings"`
	Summary       Summary      `json:"summary"`
	Hosts         []HostResult `json:"hosts"`
}

// HostInfo describes a scan target: the address dialed and the name the user gave.
type HostInfo struct {
	IP       string
	Hostname string
}

// BuildReport assembles a report from a finished (or cancelled) scan.
// hosts defines the order of the output.
func BuildReport(tool string, hosts []HostInfo, cfg Config, out Outcome, start, end time.Time) ScanReport {
	byHost := make(map[string][]PortResult, len(hosts))
	var sum Summary
	for _, r := range out.Results {
		byHost[r.Host] = append(byHost[r.Host], r)
		sum.PortsChecked++
		switch r.State {
		case StateOpen:
			sum.Open++
		case StateClosed:
			sum.Closed++
		default:
			sum.FilteredOrUnk++
		}
	}
	sum.HostsScanned = len(hosts)
	sum.PortsPlanned = out.Total

	rep := ScanReport{
		SchemaVersion: SchemaVersion,
		ScanID:        newID(),
		Tool:          tool,
		Status:        StatusComplete,
		StartedAt:     start.UTC(),
		CompletedAt:   end.UTC(),
		DurationMS:    end.Sub(start).Milliseconds(),
		Settings: Settings{
			Workers:   cfg.Workers,
			TimeoutMS: cfg.Timeout.Milliseconds(),
			Ports:     len(cfg.Ports),
			Protocol:  "tcp",
		},
		Summary: sum,
		Hosts:   make([]HostResult, 0, len(hosts)),
		Targets: make([]string, 0, len(hosts)),
	}
	if out.Incomplete {
		rep.Status = StatusIncomplete
	}
	for _, h := range hosts {
		ports := byHost[h.IP]
		sort.Slice(ports, func(i, j int) bool { return ports[i].Port < ports[j].Port })
		if ports == nil {
			ports = []PortResult{}
		}
		reachable := false
		for _, p := range ports {
			// A refused connection also proves the host answered.
			if p.State == StateOpen || p.State == StateClosed {
				reachable = true
				break
			}
		}
		target := h.IP
		if h.Hostname != "" {
			target = h.Hostname
		}
		rep.Targets = append(rep.Targets, target)
		rep.Hosts = append(rep.Hosts, HostResult{IP: h.IP, Hostname: h.Hostname, Reachable: reachable, Ports: ports})
	}
	return rep
}

func newID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
