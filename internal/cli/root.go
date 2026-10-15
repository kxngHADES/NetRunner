// Package cli handles flags, validation and command execution.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
)

// Version is the NetRunner version.
const Version = "0.1.0"

// Exit codes (see docs/usage.md).
const (
	ExitOK          = 0
	ExitInternal    = 1
	ExitUsage       = 2
	ExitOperational = 3
	ExitInterrupted = 130
)

// usageError marks invalid arguments or configuration (exit code 2).
type usageError struct{ msg string }

func (e usageError) Error() string { return e.msg }

func usagef(format string, a ...any) error { return usageError{fmt.Sprintf(format, a...)} }

const rootHelp = `NetRunner - network discovery and diagnostics (v` + Version + `)

Usage:
  netrunner <command> [flags]

Commands:
  scan      Scan TCP ports on one or more hosts
  version   Print the version

Examples:
  netrunner scan --target 127.0.0.1 --ports 1-1024
  netrunner scan --target 192.168.1.10 --ports 22,80,443,8080
  netrunner scan --target 192.168.1.10 --ports web --format json --output results.json

Only scan networks and systems you own or have explicit permission to assess.
Run "netrunner <command> --help" for command flags.
`

// Run executes NetRunner and returns the process exit code.
func Run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, rootHelp)
		return ExitUsage
	}
	switch args[0] {
	case "-h", "--help", "help":
		fmt.Fprint(stdout, rootHelp)
		return ExitOK
	case "version", "--version":
		fmt.Fprintln(stdout, "netrunner", Version)
		return ExitOK
	case "scan":
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return finish(runScan(ctx, args[1:], stdout, stderr), stderr)
	default:
		fmt.Fprintf(stderr, "netrunner: unknown command %q\n\n%s", args[0], rootHelp)
		return ExitUsage
	}
}

// exitError carries a specific exit code out of a command.
type exitError struct {
	code int
	err  error
}

func (e exitError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func finish(err error, stderr io.Writer) int {
	if err == nil {
		return ExitOK
	}
	var ee exitError
	if errors.As(err, &ee) {
		if ee.err != nil {
			fmt.Fprintln(stderr, "netrunner:", ee.err)
		}
		return ee.code
	}
	var ue usageError
	if errors.As(err, &ue) {
		fmt.Fprintln(stderr, "netrunner:", ue.msg)
		return ExitUsage
	}
	fmt.Fprintln(stderr, "netrunner:", err)
	return ExitInternal
}
