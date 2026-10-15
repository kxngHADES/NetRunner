// Package scanner performs bounded, cancellable TCP port scanning.
// It is independent of any CLI framework.
package scanner

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Config is the validated input to a scan.
type Config struct {
	Targets []string // addresses to dial (IPs after resolution)
	Ports   []uint16
	Workers int
	Timeout time.Duration
}

// Validate checks the configuration before any network operation.
func (c Config) Validate() error {
	switch {
	case len(c.Targets) == 0:
		return errors.New("no targets specified")
	case len(c.Ports) == 0:
		return errors.New("no ports specified")
	case c.Workers < 1 || c.Workers > MaxWorkers:
		return fmt.Errorf("workers must be between 1 and %d (got %d)", MaxWorkers, c.Workers)
	case c.Timeout <= 0 || c.Timeout > MaxTimeout:
		return fmt.Errorf("timeout must be greater than 0 and at most %s (got %s)", MaxTimeout, c.Timeout)
	}
	return nil
}

// Port profiles usable in place of a port list.
var profiles = map[string][]uint16{
	"common": {21, 22, 23, 25, 53, 80, 110, 135, 139, 143, 443, 445, 993, 995,
		1433, 3306, 3389, 5432, 5900, 6379, 8080, 8443},
	"web": {80, 443, 8000, 8008, 8080, 8443, 8888},
}

// DefaultProfile is used when --ports is not given.
const DefaultProfile = "common"

// ParsePorts parses specs like "22,80,443", "1-1024", "22,80,8000-8100" or
// profile names ("common", "web"). Duplicates are removed; the result is sorted.
func ParsePorts(spec string) ([]uint16, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("empty port list")
	}
	seen := make(map[uint16]struct{})
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return nil, fmt.Errorf("invalid port list %q: empty entry", spec)
		}
		if p, ok := profiles[strings.ToLower(tok)]; ok {
			for _, port := range p {
				seen[port] = struct{}{}
			}
			continue
		}
		lo, hi, isRange := strings.Cut(tok, "-")
		start, err := parsePort(lo)
		if err != nil {
			return nil, err
		}
		end := start
		if isRange {
			if end, err = parsePort(hi); err != nil {
				return nil, err
			}
			if end < start {
				return nil, fmt.Errorf("invalid port range %q: start is greater than end", tok)
			}
		}
		for p := int(start); p <= int(end); p++ {
			seen[uint16(p)] = struct{}{}
		}
	}
	ports := make([]uint16, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Slice(ports, func(i, j int) bool { return ports[i] < ports[j] })
	return ports, nil
}

func parsePort(s string) (uint16, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q: not a number", s)
	}
	if n < 1 || n > 65535 {
		return 0, fmt.Errorf("invalid port %d: must be between 1 and 65535", n)
	}
	return uint16(n), nil
}

var hostnameRE = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)

// ParseTargets validates a comma-separated list of IPv4 addresses and
// hostnames, removing duplicates. Subnets are not accepted here.
func ParseTargets(spec string) ([]string, error) {
	if strings.TrimSpace(spec) == "" {
		return nil, errors.New("no target specified")
	}
	seen := make(map[string]struct{})
	var out []string
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			return nil, fmt.Errorf("invalid target list %q: empty entry", spec)
		}
		if strings.Contains(tok, "/") {
			return nil, fmt.Errorf("invalid target %q: subnets are not supported by scan", tok)
		}
		if addr, err := netip.ParseAddr(tok); err == nil {
			if !addr.Is4() {
				return nil, fmt.Errorf("invalid target %q: only IPv4 is supported", tok)
			}
			tok = addr.String()
		} else if len(tok) > 253 || !hostnameRE.MatchString(tok) {
			return nil, fmt.Errorf("invalid target %q: not an IPv4 address or hostname", tok)
		}
		key := strings.ToLower(tok)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tok)
	}
	return out, nil
}

// Resolve turns a validated target into a HostInfo. IP literals are used as-is;
// hostnames are resolved once to an IPv4 address so the scan does not repeat
// DNS lookups for every port.
func Resolve(ctx context.Context, target string) (HostInfo, error) {
	if addr, err := netip.ParseAddr(target); err == nil {
		return HostInfo{IP: addr.String()}, nil
	}
	addrs, err := net.DefaultResolver.LookupNetIP(ctx, "ip4", target)
	if err != nil {
		return HostInfo{}, fmt.Errorf("resolving %q: %w", target, err)
	}
	if len(addrs) == 0 {
		return HostInfo{}, fmt.Errorf("resolving %q: no IPv4 address found", target)
	}
	return HostInfo{IP: addrs[0].String(), Hostname: target}, nil
}
