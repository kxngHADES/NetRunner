package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"
	"unicode"

	"github.com/kxngHADES/NetRunner/internal/scanner"
)

// TableOptions controls human-readable output.
type TableOptions struct {
	ShowClosed bool
	Color      bool
}

// Sanitize strips control characters so untrusted names or error text cannot
// inject terminal escape sequences.
func Sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
}

func colorize(on bool, state, text string) string {
	if !on {
		return text
	}
	switch state {
	case scanner.StateOpen:
		return "\x1b[32m" + text + "\x1b[0m"
	case scanner.StateClosed:
		return "\x1b[31m" + text + "\x1b[0m"
	default:
		return "\x1b[33m" + text + "\x1b[0m"
	}
}

// WriteTable writes a human-readable report.
func WriteTable(w io.Writer, rep scanner.ScanReport, opt TableOptions) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "HOST\tPORT\tSTATE\tSERVICE\tLATENCY\tNOTE\n")
	shown := 0
	for _, h := range rep.Hosts {
		name := h.IP
		if h.Hostname != "" {
			name = Sanitize(h.Hostname) + " (" + h.IP + ")"
		}
		for _, p := range h.Ports {
			if p.State != scanner.StateOpen && !opt.ShowClosed {
				continue
			}
			lat := ""
			if p.LatencyMS > 0 {
				lat = fmt.Sprintf("%.1fms", p.LatencyMS)
			}
			// tabwriter counts escape bytes as width, so colour the final-width
			// state text only; columns after it may shift slightly.
			state := colorize(opt.Color, p.State, p.State)
			fmt.Fprintf(tw, "%s\t%d/tcp\t%s\t%s\t%s\t%s\n", name, p.Port, state, p.Service, lat, Sanitize(p.Error))
			shown++
		}
	}
	if shown == 0 {
		fmt.Fprintf(tw, "(no open ports found)\t\t\t\t\t\n")
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	s := rep.Summary
	fmt.Fprintf(w, "\nScan %s: %s\n", rep.ScanID, strings.ToUpper(rep.Status))
	fmt.Fprintf(w, "Hosts scanned: %d   Ports checked: %d/%d\n", s.HostsScanned, s.PortsChecked, s.PortsPlanned)
	fmt.Fprintf(w, "Open: %d   Closed: %d   Filtered/unknown: %d\n", s.Open, s.Closed, s.FilteredOrUnk)
	fmt.Fprintf(w, "Duration: %s\n", (time.Duration(rep.DurationMS) * time.Millisecond).Round(time.Millisecond))
	if rep.Status == scanner.StatusIncomplete {
		fmt.Fprintln(w, "WARNING: scan was interrupted; results are partial.")
	}
	if s.FilteredOrUnk > 0 {
		fmt.Fprintln(w, "Note: a timeout does not prove a port is filtered; it may be packet loss or an unavailable host.")
	}
	return nil
}
