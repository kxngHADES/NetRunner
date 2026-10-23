package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kxngHADES/NetRunner/internal/output"
	"github.com/kxngHADES/NetRunner/internal/scanner"
)

const scanHelp = `Usage: netrunner scan --target <host[,host...]> [flags]

Flags:
`

func runScan(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)
	target := fs.String("target", "", "IPv4 address or hostname; comma-separated for several (required)")
	ports := fs.String("ports", scanner.DefaultProfile, "ports: 22,80 | 1-1024 | 22,8000-8100 | profile (common, web)")
	workers := fs.Int("workers", scanner.DefaultWorkers, fmt.Sprintf("maximum concurrent connections (1-%d)", scanner.MaxWorkers))
	timeout := fs.Duration("timeout", scanner.DefaultTimeout, "TCP connection timeout per attempt")
	format := fs.String("format", "table", "output format: table or json")
	outPath := fs.String("output", "", "write report to this file instead of standard output")
	noColor := fs.Bool("no-color", false, "disable terminal colors")
	quiet := fs.Bool("quiet", false, "suppress progress output")
	showClosed := fs.Bool("show-closed", false, "include closed and filtered ports in table output")
	dash := fs.Bool("dashboard", false, "show a live dashboard of workers, hosts and totals")
	fs.Usage = func() {
		fmt.Fprint(stderr, scanHelp)
		fs.PrintDefaults()
		fmt.Fprint(stderr, "\nExamples:\n  netrunner scan --target 127.0.0.1 --ports 1-1024\n"+
			"  netrunner scan --target 192.168.1.10 --ports 22,80,443 --workers 50 --timeout 800ms\n"+
			"  netrunner scan --target 192.168.1.10 --format json --output results.json\n"+
			"  netrunner scan --target 192.168.1.1,192.168.1.10 --ports 1-1024 --dashboard\n")
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
	targets, err := scanner.ParseTargets(*target)
	if err != nil {
		return usagef("--target: %v", err)
	}
	portList, err := scanner.ParsePorts(*ports)
	if err != nil {
		return usagef("--ports: %v", err)
	}
	if *format != "table" && *format != "json" {
		return usagef("--format: must be \"table\" or \"json\" (got %q)", *format)
	}
	cfg := scanner.Config{Targets: targets, Ports: portList, Workers: *workers, Timeout: *timeout}
	if err := cfg.Validate(); err != nil {
		return usagef("%v", err)
	}
	if *outPath != "" {
		if err := checkOutputPath(*outPath); err != nil {
			return usagef("--output: %v", err)
		}
	}

	// Resolve hostnames once, up front.
	hosts := make([]scanner.HostInfo, 0, len(targets))
	cfg.Targets = cfg.Targets[:0:0]
	for _, t := range targets {
		h, err := scanner.Resolve(ctx, t)
		if err != nil {
			return exitError{code: ExitOperational, err: fmt.Errorf("scan: %w", err)}
		}
		hosts = append(hosts, h)
		cfg.Targets = append(cfg.Targets, h.IP)
	}

	scope := fmt.Sprintf("target %s   ports %d   timeout %s", strings.Join(targets, ","), len(portList), cfg.Timeout)
	var board *dashboard
	if *dash && !*quiet {
		board = newDashboard(stderr, "scan", scope, "ports", cfg.Workers, len(hosts)*len(portList), !*noColor && useColor(stderr))
	}
	prog := newProgress(stderr, !*quiet && board == nil)
	onProgress := prog.update
	if board != nil {
		board.addHosts(hosts, len(portList))
		onProgress = board.onScan
	} else if !*quiet {
		fmt.Fprintf(stderr, "NetRunner v%s\nTarget: %s   Ports: %d   Workers: %d   Timeout: %s\n",
			Version, strings.Join(targets, ","), len(portList), cfg.Workers, cfg.Timeout)
	}

	start := time.Now()
	if board != nil {
		board.run()
	}
	out := scanner.Scan(ctx, cfg, scanner.CheckTCP, onProgress)
	end := time.Now()
	if board != nil {
		board.close(out.Incomplete)
	}
	prog.finish()

	rep := scanner.BuildReport("netrunner/"+Version, hosts, cfg, out, start, end)

	if err := writeReport(rep, *format, *outPath, stdout, !*noColor, *showClosed); err != nil {
		return exitError{code: ExitOperational, err: fmt.Errorf("writing report: %w", err)}
	}
	if *outPath != "" && !*quiet {
		fmt.Fprintf(stderr, "Report written to %s\n", *outPath)
	}
	if rep.Status == scanner.StatusIncomplete {
		fmt.Fprintln(stderr, "netrunner: scan interrupted; report is incomplete")
		return exitError{code: ExitInterrupted}
	}
	return nil
}

func writeReport(rep scanner.ScanReport, format, path string, stdout io.Writer, color, showClosed bool) error {
	return writeOutput(path, stdout, func(w io.Writer, toFile bool) error {
		if format == "json" {
			return output.WriteJSON(w, rep)
		}
		return output.WriteTable(w, rep, output.TableOptions{
			ShowClosed: showClosed,
			Color:      color && !toFile && useColor(stdout),
		})
	})
}

// writeOutput runs write against the file at path, or stdout when path is empty.
func writeOutput(path string, stdout io.Writer, write func(w io.Writer, toFile bool) error) error {
	if path == "" {
		return write(stdout, false)
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := write(f, true); err != nil {
		return err
	}
	return f.Close()
}

func checkOutputPath(path string) error {
	dir := filepath.Dir(path)
	st, err := os.Stat(dir)
	if err != nil || !st.IsDir() {
		return fmt.Errorf("directory %q does not exist", dir)
	}
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		return fmt.Errorf("%q is a directory", path)
	}
	return nil
}
