# NetRunner

A command-line network discovery and diagnostics tool written in Go. This is **Phase 1**: a bounded, cancellable TCP port scanner with JSON export.

> **Only scan networks and systems you own or have explicit permission to assess.**
> NetRunner is a diagnostics and inventory tool, not an exploitation framework.

## Install

```
go build -o bin/netrunner ./cmd/netrunner      # or: make build
go install ./cmd/netrunner
```

Requires Go 1.22+ (standard library only, no third-party dependencies).

## Usage

```
netrunner scan --target 127.0.0.1 --ports 1-1024
netrunner scan --target 192.168.1.10 --ports 22,80,443,8080
netrunner scan --target 192.168.1.10 --ports web --workers 50 --timeout 800ms
netrunner scan --target 192.168.1.10 --format json --output results.json
```

| Flag | Meaning | Default |
|---|---|---|
| `--target` | IPv4 address or hostname (comma-separated for several) | required |
| `--ports` | `22,80`, `1-1024`, `22,8000-8100`, or a profile (`common`, `web`) | `common` |
| `--workers` | concurrent connections (1-1000) | `100` |
| `--timeout` | per-attempt TCP timeout | `1s` |
| `--format` | `table` or `json` | `table` |
| `--output` | write report to a file | stdout |
| `--show-closed` | include closed/filtered ports in the table | off |
| `--no-color` | disable colors | auto |
| `--quiet` | suppress progress | off |

Progress goes to stderr, only on an interactive terminal, so `--format json` on stdout stays machine-readable.

## Port states

| State | Meaning |
|---|---|
| `open` | TCP connection succeeded |
| `closed` | connection explicitly refused |
| `filtered_or_unknown` | timeout or other ambiguous error (a timeout does **not** prove filtering) |

## Exit codes

| Code | Meaning |
|---|---|
| 0 | scan completed |
| 1 | unexpected internal error |
| 2 | invalid arguments or configuration |
| 3 | scan could not complete (e.g. name resolution or report write failed) |
| 130 | interrupted with Ctrl+C; the report is written with `"status": "incomplete"` |

## JSON report (schema_version 1)

Top level: `schema_version`, `scan_id`, `tool`, `status` (`complete`/`incomplete`), `started_at`, `completed_at`, `duration_ms`, `targets`, `settings`, `summary`, `hosts[]` (each with `ip`, `hostname`, `reachable`, `ports[]`). Port entries carry `latency_ms` for successful connections.

## Limitations

- TCP connect checks only; no ICMP/ARP, UDP, or stealth scanning.
- Service names come from the port number and are hints, not proof.
- IPv4 only. Subnet discovery (`discover`), live dashboard and scan history are planned for later phases.

## Development

```
make test    # go test ./...
make race    # go test -race ./...
make vet
```
