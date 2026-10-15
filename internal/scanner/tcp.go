package scanner

import (
	"context"
	"errors"
	"net"
	"strconv"
	"syscall"
	"time"

	"github.com/kxngHADES/NetRunner/internal/services"
)

// wsaeconnrefused is WSAECONNREFUSED on Windows, where syscall.ECONNREFUSED
// is a different, synthetic value.
const wsaeconnrefused = syscall.Errno(10061)

// Checker performs a single TCP connection attempt. It is a field-like
// dependency so tests can substitute a fake.
type Checker func(ctx context.Context, host string, port uint16, timeout time.Duration) PortResult

// CheckTCP dials host:port with a per-attempt timeout and classifies the result.
func CheckTCP(ctx context.Context, host string, port uint16, timeout time.Duration) PortResult {
	res := PortResult{
		Host:     host,
		Port:     port,
		Protocol: "tcp",
		Service:  services.Lookup(port),
	}

	d := net.Dialer{Timeout: timeout}
	start := time.Now()
	conn, err := d.DialContext(ctx, "tcp", net.JoinHostPort(host, strconv.Itoa(int(port))))
	elapsed := time.Since(start)

	if err == nil {
		_ = conn.Close()
		res.State = StateOpen
		res.LatencyMS = float64(elapsed.Microseconds()) / 1000
		return res
	}
	if ctx.Err() != nil {
		res.State = stateCancelled
		return res
	}
	res.State, res.Error = classify(err)
	return res
}

// classify maps a dial error to a port state and a short error category.
func classify(err error) (state, msg string) {
	if isRefused(err) {
		return StateClosed, "connection refused"
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return StateFiltered, "timeout"
	}
	return StateFiltered, "error: " + err.Error()
}

func isRefused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var errno syscall.Errno
	return errors.As(err, &errno) && errno == wsaeconnrefused
}
