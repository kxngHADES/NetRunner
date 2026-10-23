package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/kxngHADES/NetRunner/internal/output"
	"github.com/kxngHADES/NetRunner/internal/scanner"
)

const discoverHelp = `Usage: netrunner discover --subnet <cidr[,cidr...]> [flags]

Finds live hosts by attempting TCP connections to a few common ports on every
address. A host counts as alive if any probe is accepted or refused; its
remaining probes are then skipped. No ICMP or ARP is used.

Flags:
`

func runDiscover(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("discover", flag.ContinueOnError)
	fs.SetOutput(stderr)
	subnet := fs.String("subnet", "", "IPv4 subnet in CIDR form, /16 or longer; comma-separated for several (required)")
	ports := fs.String("ports", scanner.DefaultDiscoverProfile, "probe ports: 22,80 | 1-1024 | profile (discover, common, web)")
	workers := fs.Int("workers", scanner.DefaultWorkers, fmt.Sprintf("maximum concurrent connections (1-%d)", scanner.MaxWorkers))
	timeout := fs.Duration("timeout", scanner.DefaultTimeout, "TCP connection timeout per probe")
	format := fs.String("format", "table", "output format: table or json")
	outPath := fs.String("output", "", "write report to this file instead of standard output")
	noColor := fs.Bool("no-color", false, "disable terminal colors")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	dash := fs.Bool("dashboard", false, "show a live dashboard of workers, hosts and totals")
	fs.Usage = func() {
		fmt.Fprint(stderr, discoverHelp)
		fs.PrintDefaults()
		fmt.Fprint(stderr, "\nExamples:\n  netrunner discover --subnet 192.168.1.0/24\n"+
			"  netrunner discover --subnet 192.168.1.0/24 --dashboard\n"+
			"  netrunner discover --subnet 10.0.0.0/24,10.0.1.0/24 --ports 22,443 --timeout 500ms\n"+
			"  netrunner discover --subnet 192.168.1.0/24 --format json --output hosts.json\n")
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return exitError{code: ExitUsage}
	}
	if fs.NArg() > 0 {
		return usagef("unexpected argument %q", fs.Arg(0))
	}

	// Validate everything before touching the network.
	subnets, hosts, err := scanner.ParseSubnets(*subnet)
	if err != nil {
		return usagef("--subnet: %v", err)
	}
	portList, err := scanner.ParsePorts(*ports)
	if err != nil {
		return usagef("--ports: %v", err)
	}
	if *format != "table" && *format != "json" {
		return usagef("--format: must be \"table\" or \"json\" (got %q)", *format)
	}
	cfg := scanner.Config{Targets: hosts, Ports: portList, Workers: *workers, Timeout: *timeout}
	if err := cfg.Validate(); err != nil {
		return usagef("%v", err)
	}
	if *outPath != "" {
		if err := checkOutputPath(*outPath); err != nil {
			return usagef("--output: %v", err)
		}
	}

	scope := fmt.Sprintf("subnet %s   probe ports %d   timeout %s", strings.Join(subnets, ","), len(portList), cfg.Timeout)
	var board *dashboard
	if *dash && !*quiet {
		board = newDashboard(stderr, "discover", scope, "hosts", cfg.Workers, len(hosts), !*noColor && useColor(stderr))
	}
	prog := newProgress(stderr, !*quiet && board == nil)
	prog.label = "alive"
	onProgress := func(p scanner.DiscoverProgress) {
		prog.update(scanner.Progress{Done: p.HostsDone, Total: p.HostsTotal, Open: p.Alive, Active: p.Active})
	}
	if board != nil {
		onProgress = board.onDiscover
	} else if !*quiet {
		fmt.Fprintf(stderr, "NetRunner v%s\nSubnet: %s   Hosts: %d   Probe ports: %d   Workers: %d   Timeout: %s\n",
			Version, strings.Join(subnets, ","), len(hosts), len(portList), cfg.Workers, cfg.Timeout)
	}

	start := time.Now()
	if board != nil {
		board.run()
	}
	out := scanner.Discover(ctx, cfg, scanner.CheckTCP, onProgress)
	end := time.Now()
	if board != nil {
		board.close(out.Incomplete)
	}
	prog.finish()

	rep := scanner.BuildDiscoveryReport("netrunner/"+Version, subnets, cfg, out, start, end)

	err = writeOutput(*outPath, stdout, func(w io.Writer, toFile bool) error {
		if *format == "json" {
			return output.WriteJSON(w, rep)
		}
		return output.WriteDiscoveryTable(w, rep, output.TableOptions{Color: !*noColor && !toFile && useColor(stdout)})
	})
	if err != nil {
		return exitError{code: ExitOperational, err: fmt.Errorf("writing report: %w", err)}
	}
	if *outPath != "" && !*quiet {
		fmt.Fprintf(stderr, "Report written to %s\n", *outPath)
	}
	if rep.Status == scanner.StatusIncomplete {
		fmt.Fprintln(stderr, "netrunner: discovery interrupted; report is incomplete")
		return exitError{code: ExitInterrupted}
	}
	return nil
}
