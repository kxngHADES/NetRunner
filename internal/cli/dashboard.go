package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/kxngHADES/NetRunner/internal/output"
	"github.com/kxngHADES/NetRunner/internal/scanner"
)

// Dashboard layout limits. The frame stays within 78 columns and about 24
// rows so it redraws cleanly on a standard terminal.
const (
	dashWidth    = 78
	dashBarWidth = 30
	dashMaxHosts = 6
	dashMaxFeed  = 5
	dashInterval = 150 * time.Millisecond
)

// dashboard is a multi-line live view on stderr showing progress, worker
// activity, per-host state and running totals. Progress callbacks only update
// state under the lock; a ticker goroutine owns all drawing.
type dashboard struct {
	mu      sync.Mutex
	w       io.Writer
	color   bool
	kind    string // "scan" or "discover"
	scope   string // one-line description of what is being probed
	unit    string // what done/total counts: "ports" or "hosts"
	workers int
	start   time.Time

	done, total, active    int
	open, closed, filtered int
	probes, alive          int

	hosts []*dashHost // scan only, in target order
	byIP  map[string]*dashHost
	feed  []string // most recent findings, newest last
	feedN int

	lines   int
	stop    chan struct{}
	stopped chan struct{}
}

type dashHost struct {
	name        string
	ip          string
	done, total int
	open        int
	answered    bool
	finished    time.Time
}

// newDashboard returns nil when w is not an interactive terminal that can
// handle ANSI escapes; callers then fall back to the plain progress line.
func newDashboard(w io.Writer, kind, scope, unit string, workers, total int, color bool) *dashboard {
	f, ok := w.(*os.File)
	if !ok || !isTerminal(w) || !enableVT(f) {
		return nil
	}
	return &dashboard{
		w:       w,
		color:   color,
		kind:    kind,
		scope:   scope,
		unit:    unit,
		workers: workers,
		total:   total,
		start:   time.Now(),
		byIP:    make(map[string]*dashHost),
	}
}

// run starts the redraw loop.
func (d *dashboard) run() {
	d.stop = make(chan struct{})
	d.stopped = make(chan struct{})
	fmt.Fprint(d.w, "\x1b[?25l") // hide cursor while redrawing
	go func() {
		defer close(d.stopped)
		t := time.NewTicker(dashInterval)
		defer t.Stop()
		for {
			d.draw("")
			select {
			case <-t.C:
			case <-d.stop:
				return
			}
		}
	}()
}

// close stops the redraw loop and leaves the final frame on screen.
func (d *dashboard) close(incomplete bool) {
	close(d.stop)
	<-d.stopped
	status := "COMPLETE"
	if incomplete {
		status = "INTERRUPTED"
	}
	d.draw(status)
	fmt.Fprint(d.w, "\x1b[?25h\n")
}

// addHosts registers scan targets so each gets a row.
func (d *dashboard) addHosts(hosts []scanner.HostInfo, ports int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, h := range hosts {
		name := h.IP
		if h.Hostname != "" {
			name = output.Sanitize(h.Hostname)
		}
		dh := &dashHost{name: name, ip: h.IP, total: ports}
		d.hosts = append(d.hosts, dh)
		d.byIP[h.IP] = dh
	}
}

func (d *dashboard) push(line string) {
	d.feed = append(d.feed, line)
	if len(d.feed) > dashMaxFeed {
		d.feed = d.feed[len(d.feed)-dashMaxFeed:]
	}
	d.feedN++
}

func latency(ms float64) string {
	if ms <= 0 {
		return ""
	}
	return fmt.Sprintf("%.1fms", ms)
}

// onScan is the scanner.Scan progress callback.
func (d *dashboard) onScan(p scanner.Progress) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.done, d.active = p.Done, p.Active
	r := p.Result
	switch r.State {
	case scanner.StateOpen:
		d.open++
	case scanner.StateClosed:
		d.closed++
	default:
		d.filtered++
	}
	h := d.byIP[r.Host]
	if h == nil {
		return
	}
	h.done++
	if r.State == scanner.StateOpen || r.State == scanner.StateClosed {
		h.answered = true
	}
	if h.done == h.total {
		h.finished = time.Now()
	}
	if r.State == scanner.StateOpen {
		h.open++
		d.push(fmt.Sprintf("%-22s %-10s %-12s %s", fit(h.name, 22), fmt.Sprintf("%d/tcp", r.Port), r.Service, latency(r.LatencyMS)))
	}
}

// onDiscover is the scanner.Discover progress callback.
func (d *dashboard) onDiscover(p scanner.DiscoverProgress) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.done, d.active, d.probes, d.alive = p.HostsDone, p.Active, p.Probes, p.Alive
	if h := p.Found; h != nil {
		d.push(fmt.Sprintf("%-22s %-10s %-8s %-12s %s", h.IP, fmt.Sprintf("%d/tcp", h.Port), h.State, h.Service, latency(h.LatencyMS)))
	}
}

// fit truncates or pads s to exactly n runes.
func fit(s string, n int) string {
	if utf8.RuneCountInString(s) > n {
		r := []rune(s)
		return string(r[:n-1]) + "~"
	}
	return s + strings.Repeat(" ", n-utf8.RuneCountInString(s))
}

func (d *dashboard) paint(code, s string) string {
	if !d.color {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (d *dashboard) bar(n, total, width int, code string) string {
	filled := 0
	if total > 0 {
		filled = n * width / total
	}
	if filled > width {
		filled = width
	}
	return d.paint(code, strings.Repeat("█", filled)) + d.paint("2", strings.Repeat("░", width-filled))
}

func clock(t time.Duration) string {
	t = t.Round(time.Second)
	return fmt.Sprintf("%02d:%02d", int(t.Minutes()), int(t.Seconds())%60)
}

// frame renders the current state as lines. Visible text in every line is at
// most dashWidth columns so the terminal never wraps and redraws stay aligned.
func (d *dashboard) frame(status string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()

	elapsed := time.Since(d.start)
	var lines []string
	add := func(format string, a ...any) { lines = append(lines, fmt.Sprintf(format, a...)) }

	state := status
	if state == "" {
		state = "RUNNING"
	}
	stateCode := "36"
	switch state {
	case "COMPLETE":
		stateCode = "32"
	case "INTERRUPTED":
		stateCode = "33"
	}
	title := fmt.Sprintf(" NETRUNNER v%s  %s", Version, d.kind)
	right := fmt.Sprintf("%s  %s ", state, clock(elapsed))
	pad := dashWidth - utf8.RuneCountInString(title) - utf8.RuneCountInString(right)
	if pad < 1 {
		pad = 1
	}
	add("%s%s%s", d.paint("1;36", title), strings.Repeat(" ", pad), d.paint("1;"+stateCode, right))
	add(" %s", d.paint("2", fit(d.scope, dashWidth-1)))
	add("")

	pct := 0
	if d.total > 0 {
		pct = d.done * 100 / d.total
	}
	add(" Progress  %s %3d%%  %d/%d %s", d.bar(d.done, d.total, dashBarWidth, "32"), pct, d.done, d.total, d.unit)
	add(" Workers   %s %d/%d busy", d.bar(d.active, d.workers, dashBarWidth, "33"), d.active, d.workers)
	rate := 0.0
	if s := elapsed.Seconds(); s > 0 {
		rate = float64(d.done) / s
	}
	eta := "--:--"
	if rate > 0 && d.done < d.total && status == "" {
		eta = clock(time.Duration(float64(d.total-d.done) / rate * float64(time.Second)))
	}
	add(" Rate      %.1f %s/s   ETA %s", rate, d.unit, eta)
	add("")

	if d.kind == "scan" {
		lines = append(lines, d.hostRows()...)
		add("")
		add(" Totals    %s   %s   %s",
			d.paint("32", fmt.Sprintf("open %d", d.open)),
			d.paint("31", fmt.Sprintf("closed %d", d.closed)),
			d.paint("33", fmt.Sprintf("filtered/unknown %d", d.filtered)))
		add("")
		add(" %s", d.paint("1", fmt.Sprintf("Open ports (%d)", d.feedN)))
	} else {
		add(" Totals    %s   probed %d/%d hosts   probes sent %d",
			d.paint("32", fmt.Sprintf("alive %d", d.alive)), d.done, d.total, d.probes)
		add("")
		add(" %s", d.paint("1", fmt.Sprintf("Live hosts (%d)", d.feedN)))
	}
	if len(d.feed) == 0 {
		add("   %s", d.paint("2", "none yet"))
	}
	if more := d.feedN - len(d.feed); more > 0 {
		add("   %s", d.paint("2", fmt.Sprintf("... %d earlier", more)))
	}
	for _, f := range d.feed {
		add("   %s %s", d.paint("32", "+"), strings.TrimRight(fit(f, dashWidth-5), " "))
	}
	return lines
}

// hostRows renders the per-host table, preferring hosts in progress, then
// queued ones, then the most recently finished.
func (d *dashboard) hostRows() []string {
	var running, queued, finished []*dashHost
	for _, h := range d.hosts {
		switch {
		case h.done >= h.total:
			finished = append(finished, h)
		case h.done > 0:
			running = append(running, h)
		default:
			queued = append(queued, h)
		}
	}
	sort.SliceStable(finished, func(i, j int) bool { return finished[i].finished.After(finished[j].finished) })
	show := append(append(append([]*dashHost{}, running...), queued...), finished...)
	if len(show) > dashMaxHosts {
		show = show[:dashMaxHosts]
	}

	rows := []string{" " + d.paint("1", fmt.Sprintf("%-24s %-14s %-11s %4s  %s", "HOST", "PROGRESS", "PORTS", "OPEN", "STATUS"))}
	for _, h := range show {
		status := d.paint("2", "queued")
		switch {
		case h.done >= h.total && h.answered:
			status = d.paint("32", "done")
		case h.done >= h.total:
			status = d.paint("33", "no answer")
		case h.done > 0:
			status = d.paint("36", "scanning")
		}
		open := fmt.Sprintf("%4d", h.open)
		if h.open > 0 {
			open = d.paint("32", open)
		}
		rows = append(rows, fmt.Sprintf(" %s %s   %-11s %s  %s",
			fit(h.name, 24), d.bar(h.done, h.total, 11, "32"), fmt.Sprintf("%d/%d", h.done, h.total), open, status))
	}
	if hidden := len(d.hosts) - len(show); hidden > 0 {
		rows = append(rows, " "+d.paint("2", fmt.Sprintf("... %d more hosts (%d finished)", hidden, len(finished))))
	}
	return rows
}

// draw repaints the frame in place, overwriting the previous one.
func (d *dashboard) draw(status string) {
	lines := d.frame(status)
	var b strings.Builder
	if d.lines > 0 {
		fmt.Fprintf(&b, "\x1b[%dF", d.lines) // back to the first line of the last frame
	}
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\x1b[K\n")
	}
	b.WriteString("\x1b[J")
	d.lines = len(lines)
	io.WriteString(d.w, b.String())
}
