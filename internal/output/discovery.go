package output

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kxngHADES/NetRunner/internal/scanner"
)

// WriteDiscoveryTable writes a human-readable discovery report.
func WriteDiscoveryTable(w io.Writer, rep scanner.DiscoveryReport, opt TableOptions) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "HOST\tANSWERED ON\tSERVICE\tLATENCY\n")
	for _, h := range rep.Hosts {
		lat := ""
		if h.LatencyMS > 0 {
			lat = fmt.Sprintf("%.1fms", h.LatencyMS)
		}
		evidence := fmt.Sprintf("%d/tcp %s", h.Port, colorize(opt.Color, h.State, h.State))
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", h.IP, evidence, h.Service, lat)
	}
	if len(rep.Hosts) == 0 {
		fmt.Fprintf(tw, "(no live hosts found)\t\t\t\n")
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	s := rep.Summary
	fmt.Fprintf(w, "\nDiscovery %s: %s\n", rep.ScanID, strings.ToUpper(rep.Status))
	fmt.Fprintf(w, "Subnets: %s\n", strings.Join(rep.Subnets, ", "))
	fmt.Fprintf(w, "Live hosts: %d   Hosts probed: %d/%d   Probes sent: %d\n", s.HostsAlive, s.HostsProbed, s.HostsPlanned, s.ProbesSent)
	fmt.Fprintf(w, "Duration: %s\n", (time.Duration(rep.DurationMS) * time.Millisecond).Round(time.Millisecond))
	if rep.Status == scanner.StatusIncomplete {
		fmt.Fprintln(w, "WARNING: discovery was interrupted; results are partial.")
	}
	fmt.Fprintln(w, "Note: hosts that ignore every probe port do not appear; TCP discovery is not exhaustive.")
	return nil
}
