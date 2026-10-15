# NetRunner: Network Discovery & Intelligence Tool

Project specification · Go · CLI · Systems programming

## 1. Project overview

NetRunner is a command-line network discovery and diagnostics tool written in Go. It discovers reachable devices on an authorized local network, checks their TCP ports, identifies common services, and produces structured reports about the network.

The project is designed to start as a simple port scanner and evolve into a polished network intelligence tool with concurrent scanning, live terminal output, historical comparisons, and configurable scan profiles.

[NetsCLI — Network Scanner for Desktop, CLI, and MCP](https://images.openai.com/static-rsc-4/MWB3_qUPQ-H8VNe8lMhEO-nKl9dlsQnVGYFLskXWHMt8p-S6TcpW9CFXorVBqhRb8PQR7aFOuqnz3XZM6qx4-I_hQ7S68fA-UmYJH1HmbeP_efyaIqJl2WkoeaBH8renwy3oSyvX6yd6yVEruChm_TBh9X1CuJoAVTeeroJiKxc?purpose=inline)

[netscli.com](https://netscli.com/)

### Core objectives

- Discover hosts reachable on a user-specified local subnet.
- Identify open, closed, and filtered TCP ports where possible.
- Detect common services running on discovered ports.
- Use Go concurrency to scan multiple hosts and ports efficiently.
- Display live scan progress in a readable terminal interface.
- Export scan results to JSON and human-readable reports.
- Compare scans over time to identify changes in the network.

### Target users

- Developers learning Go and systems programming.
- Homelab administrators monitoring their own networks.
- IT technicians performing authorized network diagnostics.
- Students exploring TCP/IP, concurrency, and service discovery.

### Project boundaries

NetRunner is a network diagnostics and inventory tool, not an exploitation framework. Its initial release focuses on TCP connection checks and basic service identification. It will not attempt password cracking, stealth scanning, vulnerability exploitation, or unauthorized access.

Only scan networks and systems you own or have explicit permission to assess.

## 2. Scope and release plan

### Phase 1: Core scanner

MVP

Implement a TCP port scanner for a specified host or IP range, with configurable ports, timeouts, concurrency, and JSON output.

### Phase 2: Network discovery

Discover reachable hosts on a specified IPv4 subnet and build a device inventory with IP addresses, hostnames where available, and open ports.

### Phase 3: Live terminal dashboard

Show scan progress, active workers, discovered hosts, open ports, elapsed time, and completion statistics.

### Phase 4: Network intelligence

Add service banners for selected protocols, saved scan history, change detection, and comparison reports.

## 3. Functional requirements

The following requirements define what NetRunner must do. The identifiers can later be used in GitHub issues and acceptance tests.

### FR-01: Scan target selection

The user must be able to scan a single host, a list of hosts, or an explicitly specified IPv4 CIDR subnet.

Examples:

```
# Scan one host
netrunner scan --target 192.168.1.10

# Scan selected ports
netrunner scan --target 192.168.1.10 --ports 22,80,443,8080

# Scan a port range
netrunner scan --target 192.168.1.10 --ports 1-1024

# Discover devices on an authorized subnet
netrunner discover --subnet 192.168.1.0/24
```

Requirements:

- Validate IPv4 addresses, CIDR notation, and port numbers.
- Reject port numbers outside `1-65535`.
- Reject malformed targets and empty port lists.
- Require explicit targets. Never automatically scan arbitrary public IP ranges.
- Provide a `--help` command with examples.

### FR-02: TCP port scanning

For each target port, NetRunner must attempt a TCP connection using a configurable timeout.

| Result                | Meaning                                                              |
| --------------------- | -------------------------------------------------------------------- |
| `open`                | TCP connection succeeded                                             |
| `closed`              | The connection was explicitly refused                                |
| `filtered_or_unknown` | The connection timed out or another ambiguous network error occurred |

A timeout does not prove that a port is filtered. Firewalls, packet loss, routing issues, and an unavailable host can produce similar results.

Requirements:

- Configure connection timeout per attempt.
- Avoid waiting indefinitely for unresponsive hosts.
- Record the time taken for successful connections.
- Distinguish connection refusal from timeout and other errors.
- Close all sockets promptly.
- Respect cancellation when a user presses `Ctrl+C`.

### FR-03: Concurrent scanning

NetRunner must use a bounded worker pool rather than creating an unlimited goroutine for every port.

The user can configure concurrency:

```
netrunner scan \
	--target 192.168.1.10 \
	--ports 1-1024 \
	--workers 100 \
	--timeout 800ms
```

Requirements:

- Default worker count: 100.
- Configurable worker count with a safe upper limit.
- Workers receive scan jobs from a channel.
- Results are collected through a results channel or synchronized collector.
- Errors in one port scan must not crash the entire scan.
- Shutdown must stop workers cleanly.
- The scanner must support context cancellation and avoid goroutine leaks.

### FR-04: Host discovery

The discovery command must enumerate candidate addresses within a user-provided IPv4 subnet.

For the MVP, discovery can attempt TCP connections to a configurable set of common ports. A host that accepts a connection is considered reachable.

Requirements:

- Enumerate addresses correctly for the supplied CIDR.
- Apply concurrency and timeout limits.
- Deduplicate discovered hosts.
- Support a configurable set of discovery ports.
- Report hosts that responded without claiming that all other hosts are offline.

Important: TCP-based discovery can miss devices that have no listening ports or whose firewalls block the probes. Later, an optional ICMP or local-network ARP discovery module can improve coverage where the operating system and network permissions allow it.

### FR-05: Service identification

NetRunner should map known TCP port numbers to likely services.

Examples:

| Port | Conventional service |
| ---- | -------------------- |
| 22   | SSH                  |
| 25   | SMTP                 |
| 53   | DNS over TCP         |
| 80   | HTTP                 |
| 110  | POP3                 |
| 143  | IMAP                 |
| 443  | HTTPS                |
| 3306 | MySQL                |
| 5432 | PostgreSQL           |
| 6379 | Redis                |
| 8080 | HTTP alternate       |

Port mappings are hints, not proof. A service can run on a nonstandard port.

Requirements:

- Maintain a local service registry.
- Identify services using port metadata first.
- Add optional protocol-specific checks in a later release.
- Only collect banners for explicitly supported protocols.
- Bound response sizes and read timeouts.
- Never send arbitrary payloads to unknown services.

### FR-06: Live terminal output

The CLI should show useful information while a scan is running.

Example mock output:

```
NetRunner v0.1.0
Target: 192.168.1.0/24
Mode:   TCP discovery + port scan

[################........]  62%
Hosts discovered:  8
Ports checked:     2,145
Open ports:        14
Workers active:    100
Elapsed:           00:00:08

192.168.1.10  22/tcp   open   ssh
192.168.1.10  80/tcp   open   http
192.168.1.15  443/tcp  open   https
```

Requirements:

- Update progress without flooding the terminal with new lines.
- Display a normal, readable fallback when output is redirected to a file.
- Use terminal colors only when supported.
- Print final totals and a clear scan status.
- Avoid writing progress updates to standard output when machine-readable JSON is requested; use standard error for progress.

### FR-07: Export reports

NetRunner must export results to JSON in the MVP.

```
netrunner scan \
	--target 192.168.1.10 \
	--ports 1-1024 \
	--format json \
	--output results.json
```

Requirements:

- Include scan metadata, target information, timestamps, duration, and results.
- Use a documented and versioned JSON schema.
- Create valid JSON even when individual connection attempts fail.
- Return a nonzero exit code for invalid arguments or fatal scan failures.
- Support human-readable terminal output independently of JSON export.

### FR-08: Scan history and change detection

This feature belongs in a later release.

NetRunner will save completed scans in a local SQLite database and compare a new scan with the most recent compatible scan.

It should identify:

- Newly discovered hosts.
- Hosts no longer detected.
- Newly open TCP ports.
- Ports that were previously open but are now closed or unresponsive.
- Changes in observed service information.

Comparisons must distinguish actual observations from missing or inconclusive results. A timed-out connection must not automatically be interpreted as a port having closed.

## 4. Command-line interface specification

Use a consistent command structure so new functionality can be added without redesigning the CLI.

### Command hierarchy

`netrunner`

Root command, version, help and configuration

`scan`

Scan hosts and TCP ports

`discover`

Find responsive devices in a CIDR subnet

`report`

Inspect and compare saved scan results

`config`

Display or validate local configuration

### Proposed command flags

| Flag         | Purpose                       | Default                 |
| ------------ | ----------------------------- | ----------------------- |
| `--target`   | Hostname or IP address        | Required for `scan`     |
| `--subnet`   | IPv4 CIDR range               | Required for `discover` |
| `--ports`    | Port list or range            | Common ports profile    |
| `--workers`  | Maximum concurrent operations | `100`                   |
| `--timeout`  | TCP connection timeout        | `1s`                    |
| `--format`   | `table` or `json`             | `table`                 |
| `--output`   | Report file path              | Standard output         |
| `--no-color` | Disable terminal colors       | Automatic               |
| `--quiet`    | Suppress progress output      | `false`                 |

The `--ports` flag should accept values such as `22,80,443`, `1-1024`, and `22,80,8000-8100`. The parser must validate the input and remove duplicate ports before scheduling jobs.

The command should also support a small number of named profiles, such as `common` and `web`, instead of forcing users to remember port lists.

## 5. Technical architecture

Use a modular Go application with a clear separation between command handling, scanning logic, network operations, and output formatting.

CLI layer

Arguments · Validation · User feedback

Application layer

Scan orchestration · Context · Progress · Results

Discovery

Target enumeration and reachability

Worker pool

Bounded concurrent TCP checks

Storage

JSON files initially, SQLite later

Reporting

Tables, JSON and comparisons

### Recommended technology stack

| Component          | Recommendation                  | Reason                                        |
| ------------------ | ------------------------------- | --------------------------------------------- |
| Language           | Go 1.25+                        | Concurrency, networking and portable binaries |
| CLI framework      | Cobra                           | Commands, flags and help generation           |
| Terminal UI        | Bubble Tea, optionally          | Live interactive terminal dashboard           |
| Styling            | Lip Gloss, optionally           | Terminal layout and colors                    |
| Networking         | Standard `net` package          | TCP connections and timeouts                  |
| Address parsing    | Standard `net/netip`            | IP and CIDR validation                        |
| Configuration      | Standard library initially      | Keep the MVP dependency-light                 |
| JSON               | `encoding/json`                 | Structured export                             |
| Historical storage | SQLite, later                   | Local scan history                            |
| Testing            | `testing` and `net`             | Unit and integration tests                    |
| Distribution       | Go binaries and GitHub Releases | Simple installation across platforms          |

Use the Go standard library for the core scanner. Introduce terminal UI and database dependencies only when the corresponding features are needed. Pin a supported Go toolchain version in the repository and CI, then update it deliberately.

## 6. Proposed repository structure

Use a Go module with an application entry point, internal packages, and a separate location for tests and test fixtures.

```
netrunner/
├── cmd/
│   └── netrunner/
│       └── main.go
├── internal/
│   ├── cli/
│   │   ├── root.go
│   │   ├── scan.go
│   │   ├── discover.go
│   │   ├── report.go
│   │   └── config.go
│   ├── scanner/
│   │   ├── scanner.go
│   │   ├── worker_pool.go
│   │   ├── tcp.go
│   │   └── results.go
│   ├── discovery/
│   │   ├── cidr.go
│   │   └── discovery.go
│   ├── services/
│   │   └── registry.go
│   ├── output/
│   │   ├── table.go
│   │   └── json.go
│   ├── config/
│   │   └── config.go
│   └── history/
│       ├── store.go
│       └── compare.go
├── tests/
│   ├── integration/
│   └── fixtures/
├── docs/
│   ├── architecture.md
│   ├── usage.md
│   └── report-schema.md
├── .github/
│   └── workflows/
│       └── ci.yml
├── .gitignore
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

The `history` package is reserved for a later phase. You do not need to create every file on day one.

Architecture rules:

- `cmd/netrunner/main.go` should only initialize and run the application.
- The `cli` package handles flags, argument validation, and command execution.
- The `scanner` package owns scanning behavior, worker coordination, and TCP results.
- The `discovery` package generates targets and determines reachability.
- The `services` package contains service metadata and optional protocol identification.
- The `output` package formats results without performing network operations.
- The `history` package persists and compares scans when that feature is introduced.

Keep the core scanner independent of Cobra and Bubble Tea. This allows you to reuse it later in a GUI or HTTP API without rewriting the networking logic.

## 7. Core data models

These are the principal types the implementation should use.

### Scan configuration

```
type ScanConfig struct {
	Targets []string
	Ports []uint16
	Workers int
	Timeout time.Duration
}
```

The application layer validates this configuration before starting any network operations.

### Port scan result

```
type PortResult struct {
	Host string `json:"host"`
	Port uint16 `json:"port"`
	Protocol string `json:"protocol"`
	State string `json:"state"`
	Service string `json:"service,omitempty"`
	Latency time.Duration `json:"latency,omitempty"`
	Error string `json:"error,omitempty"`
}
```

Use a controlled set of states, such as `open`, `closed`, and `filtered_or_unknown`. If needed, store a separate error category to distinguish timeouts, refused connections, and other failures.

### Host result

```
type HostResult struct {
	IP string `json:"ip"`
	Hostname string `json:"hostname,omitempty"`
	Reachable bool `json:"reachable"`
	Ports []PortResult `json:"ports"`
}
```

A hostname should only be populated when name resolution succeeds. Do not treat a failed reverse DNS lookup as a failed host scan.

### Scan report

```
type ScanReport struct {
	SchemaVersion int `json:"schema_version"`
	ScanID string `json:"scan_id"`
	StartedAt time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at"`
	Targets []string `json:"targets"`
	Hosts []HostResult `json:"hosts"`
}
```

Add aggregate counts, scan settings, and failure information as the report format matures. Keep the schema versioned so that future releases can evolve the format without silently breaking consumers.

## 8. Worker-pool design

Concurrency is one of the most important learning objectives in this project.

The scanner should follow this processing model:

\#chatgpt-mermaid-\_r_p4\_{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;fill:rgb(237, 237, 237);}@keyframes edge-animation-frame{from{stroke-dashoffset:0;}}@keyframes dash{to{stroke-dashoffset:0;}}#chatgpt-mermaid-\_r_p4\_ .edge-animation-slow{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 50s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_p4\_ .edge-animation-fast{stroke-dasharray:9,5!important;stroke-dashoffset:900;animation:dash 20s linear infinite;stroke-linecap:round;}#chatgpt-mermaid-\_r_p4\_ .error-icon{fill:rgb(48, 48, 48);}#chatgpt-mermaid-\_r_p4\_ .error-text{fill:rgb(237, 237, 237);stroke:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .edge-thickness-normal{stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .edge-thickness-thick{stroke-width:3.5px;}#chatgpt-mermaid-\_r_p4\_ .edge-pattern-solid{stroke-dasharray:0;}#chatgpt-mermaid-\_r_p4\_ .edge-thickness-invisible{stroke-width:0;fill:none;}#chatgpt-mermaid-\_r_p4\_ .edge-pattern-dashed{stroke-dasharray:3;}#chatgpt-mermaid-\_r_p4\_ .edge-pattern-dotted{stroke-dasharray:2;}#chatgpt-mermaid-\_r_p4\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_p4\_ .marker.cross{stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_p4\_ svg{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:16px;}#chatgpt-mermaid-\_r_p4\_ p{margin:0;}#chatgpt-mermaid-\_r_p4\_ .label{font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .cluster-label text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .cluster-label span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .cluster-label span p{background-color:transparent;}#chatgpt-mermaid-\_r_p4\_ .label text,#chatgpt-mermaid-\_r_p4\_ span{fill:rgb(237, 237, 237);color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .node rect,#chatgpt-mermaid-\_r_p4\_ .node circle,#chatgpt-mermaid-\_r_p4\_ .node ellipse,#chatgpt-mermaid-\_r_p4\_ .node polygon,#chatgpt-mermaid-\_r_p4\_ .node path{fill:rgb(9, 23, 44);stroke:rgb(31, 78, 148);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .rough-node .label text,#chatgpt-mermaid-\_r_p4\_ .node .label text,#chatgpt-mermaid-\_r_p4\_ .image-shape .label,#chatgpt-mermaid-\_r_p4\_ .icon-shape .label{text-anchor:middle;}#chatgpt-mermaid-\_r_p4\_ .node .katex path{fill:#000;stroke:#000;stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .rough-node .label,#chatgpt-mermaid-\_r_p4\_ .node .label,#chatgpt-mermaid-\_r_p4\_ .image-shape .label,#chatgpt-mermaid-\_r_p4\_ .icon-shape .label{text-align:center;}#chatgpt-mermaid-\_r_p4\_ .node.clickable{cursor:pointer;}#chatgpt-mermaid-\_r_p4\_ .root .anchor path{fill:rgb(175, 175, 175)!important;stroke-width:0;stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_p4\_ .arrowheadPath{fill:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_p4\_ .edgePath .path{stroke:rgb(175, 175, 175);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .flowchart-link{stroke:rgb(175, 175, 175);fill:none;}#chatgpt-mermaid-\_r_p4\_ .edgeLabel{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_p4\_ .edgeLabel p{background-color:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_p4\_ .edgeLabel rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_p4\_ .labelBkg{background-color:rgba(0, 0, 0, 0.5);}#chatgpt-mermaid-\_r_p4\_ .cluster rect{fill:rgb(48, 48, 48);stroke:rgba(255, 255, 255, 0.15);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .cluster text{fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ .cluster span{color:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ div.mermaidTooltip{position:absolute;text-align:center;max-width:200px;padding:2px;font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";font-size:12px;background:rgb(48, 48, 48);border:1px solid rgba(255, 255, 255, 0.15);border-radius:2px;pointer-events:none;z-index:100;}#chatgpt-mermaid-\_r_p4\_ .flowchartTitleText{text-anchor:middle;font-size:18px;fill:rgb(237, 237, 237);}#chatgpt-mermaid-\_r_p4\_ rect.text{fill:none;stroke-width:0;}#chatgpt-mermaid-\_r_p4\_ .icon-shape,#chatgpt-mermaid-\_r_p4\_ .image-shape{background-color:rgb(0, 0, 0);text-align:center;}#chatgpt-mermaid-\_r_p4\_ .icon-shape p,#chatgpt-mermaid-\_r_p4\_ .image-shape p{background-color:rgb(0, 0, 0);padding:2px;}#chatgpt-mermaid-\_r_p4\_ .icon-shape .label rect,#chatgpt-mermaid-\_r_p4\_ .image-shape .label rect{opacity:0.5;background-color:rgb(0, 0, 0);fill:rgb(0, 0, 0);}#chatgpt-mermaid-\_r_p4\_ .label-icon{display:inline-block;height:1em;overflow:visible;vertical-align:-0.125em;}#chatgpt-mermaid-\_r_p4\_ .node .label-icon path{fill:currentColor;stroke:revert;stroke-width:revert;}#chatgpt-mermaid-\_r_p4\_ .node .neo-node{stroke:rgb(31, 78, 148);}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node rect,#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].cluster rect,#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node polygon{stroke:url(#chatgpt-mermaid-\_r_p4\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].swimlane.cluster rect{filter:none;}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node path{stroke:url(#chatgpt-mermaid-\_r_p4\_-gradient);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node .outer-path{filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node .neo-line path{stroke:rgb(31, 78, 148);filter:none;}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node circle{stroke:url(#chatgpt-mermaid-\_r_p4\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].node circle .state-start{fill:#000000;}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].icon-shape .icon{fill:url(#chatgpt-mermaid-\_r_p4\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_p4\_ [data-look="neo"].icon-shape .icon-neo path{stroke:url(#chatgpt-mermaid-\_r_p4\_-gradient);filter:drop-shadow( 1px 2px 2px rgba(185,185,185,1));}#chatgpt-mermaid-\_r_p4\_ .node text{font-size:14px;font-weight:600;letter-spacing:normal;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_p4\_ .edgeLabels text{font-size:13px;font-weight:600;letter-spacing:-0.08px;fill:rgb(153, 206, 255);}#chatgpt-mermaid-\_r_p4\_ .node tspan[font-weight="normal"],#chatgpt-mermaid-\_r_p4\_ .edgeLabels tspan[font-weight="normal"]{font-weight:600;}#chatgpt-mermaid-\_r_p4\_ .edgeLabel .label rect{opacity:1;rx:13px;ry:13px;fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .node rect,#chatgpt-mermaid-\_r_p4\_ .node circle,#chatgpt-mermaid-\_r_p4\_ .node ellipse,#chatgpt-mermaid-\_r_p4\_ .node polygon,#chatgpt-mermaid-\_r_p4\_ .node path{fill:rgb(0, 40, 77);stroke:rgba(255, 255, 255, 0.1);stroke-width:1px;}#chatgpt-mermaid-\_r_p4\_ .node rect{rx:16px;ry:16px;}#chatgpt-mermaid-\_r_p4\_ .node.mermaid-decision .label-container{fill:rgb(0, 14, 26);stroke:rgb(26, 62, 95);stroke-dasharray:2,2;}#chatgpt-mermaid-\_r_p4\_ .edgePaths .flowchart-link{stroke:rgb(175, 175, 175);stroke-width:1px;stroke-linecap:round;stroke-linejoin:round;}#chatgpt-mermaid-\_r_p4\_ .marker{fill:rgb(175, 175, 175);stroke:rgb(175, 175, 175);}#chatgpt-mermaid-\_r_p4\_ :root{--mermaid-font-family:-apple-system-body,ui-sans-serif,-apple-system,system-ui,"Segoe UI",Helvetica,"Apple Color Emoji",Arial,sans-serif,"Segoe UI Emoji","Segoe UI Symbol";}Validated scan configurationGenerate host-port jobsBuffered jobs channelBounded worker poolPerform TCP connection checkClassify resultResults channelAggregate resultsRender terminal outputExport JSON

### Required behavior

1. Generate a job for each requested host and port combination.
2. Start a bounded number of workers.
3. Give each connection attempt its own timeout.
4. Send results to a central collector.
5. Update progress as jobs finish.
6. Cancel outstanding work when the parent context is cancelled.
7. Close channels in a controlled order and wait for all workers to exit.

Do not use a goroutine per port with no concurrency limit. That approach can exhaust file descriptors, sockets, and memory when the target range becomes large.

### Performance expectations

Treat these as initial engineering targets to validate, not guaranteed performance figures.

| Test scenario           | Initial target                                             |
| ----------------------- | ---------------------------------------------------------- |
| Scan 20 local TCP ports | Complete within 5 seconds on a responsive test environment |
| Scan 1,000 ports        | Finish promptly when most ports refuse connections         |
| Scan unresponsive ports | Respect the configured timeout and worker limit            |
| Cancel a running scan   | Stop scheduling work and exit cleanly                      |
| Repeated scans          | No steadily growing goroutine count or resource leaks      |

Actual scan duration depends on connection behavior, operating system limits, network conditions, and the number of timeouts.

## 9. Configuration specification

NetRunner should support command-line flags first. A configuration file can be added once the defaults become more extensive.

Example configuration:

```
workers: 100
timeout: 1s
format: table
ports_profile: common

discovery:
  ports:
    - 22
    - 80
    - 443
    - 445
    - 3389

output:
  color: auto
  show_closed: false
```

The configuration system should:

- Apply defaults first, then configuration-file values, then explicit CLI overrides.
- Validate all settings before scanning.
- Reject invalid worker counts and timeouts.
- Avoid silently ignoring unknown configuration keys.
- Never store passwords, authentication tokens, or unrelated secrets.

The sample profile is a starting point. The final common-port list should be documented and version-controlled.

## 10. Error handling and exit codes

NetRunner should make failures understandable and scriptable.

| Exit code | Meaning                                                  |
| --------- | -------------------------------------------------------- |
| `0`       | Scan completed successfully                              |
| `1`       | Unexpected internal or fatal runtime error               |
| `2`       | Invalid arguments or configuration                       |
| `3`       | Scan could not complete due to a fatal operational error |
| `130`     | User interrupted the scan                                |

Individual timeouts and refused connections are expected scan outcomes, not necessarily fatal application errors.

The implementation should also:

- Include the target and operation in error messages.
- Avoid printing duplicate errors for every port on an unreachable host.
- Send diagnostics to standard error.
- Preserve partial results if cancellation occurs after some work has completed.
- Make the exit-code policy explicit in the documentation.

If the scan is cancelled, the report should clearly indicate that it is incomplete rather than presenting it as a successful full scan.

## 11. Security and reliability requirements

Even a basic port scanner needs operational safeguards.

- Explicit scope: Scan only the targets supplied by the user.
- Concurrency limits: Cap simultaneous network operations.
- Input validation: Validate hostnames, IP addresses, CIDR ranges, ports, and output paths.
- Resource cleanup: Close connections, files, and channels correctly.
- Cancellation: Propagate `context.Context` through long-running operations.
- Bounded reads: Limit protocol-banner bytes and response time.
- Data privacy: Do not upload scan results to an external service by default.
- Safe output: Avoid allowing untrusted hostnames or banners to inject terminal control sequences.
- No privileged operations by default: The MVP should not require administrator access.
- Clear scope reporting: Include the target range and scan settings in every report.

For the initial implementation, use ordinary TCP connection checks. Add raw sockets, packet crafting, or more advanced discovery only if a later, clearly defined requirement justifies the additional complexity.

## 12. Testing strategy

Testing must verify both networking behavior and the correctness of concurrent execution.

Unit tests

Test port parsing, CIDR enumeration, service mappings, configuration validation, state classification, JSON serialization, and result aggregation.

Integration tests

Start a local TCP listener on an ephemeral port and confirm that NetRunner detects it as open. Test refused connections, timeouts, and cancellation.

Concurrency tests

Verify the worker limit, safe shutdown, result counts, and absence of data races. Run `go test -race ./...` in CI where supported.

CLI tests

Check invalid arguments, help output, exit codes, quiet mode, JSON output, and cancellation from the command line.

Do not make integration tests depend on public websites or external networks. Local listeners and deterministic fixtures make tests repeatable and safer.

## 13. Definition of done for the MVP

The first release is complete when all the following criteria are met.

### MVP acceptance checklist

0 of 11

Install and run NetRunner using documented Go commands.

Scan one explicitly specified host and a configurable TCP port list.

Parse individual ports and port ranges, including duplicate removal.

Limit concurrency and respect per-connection timeouts.

Correctly distinguish successful connections, refused connections, and ambiguous failures.

Cancel cleanly with Ctrl+C and avoid goroutine leaks.

Display readable terminal results and useful scan totals.

Export a versioned JSON report to a file.

Pass unit and local-network integration tests.

Pass go vet ./... and go test -race ./... where supported.

Provide a README with installation, usage, limitations, and authorized-use guidance.

Reset checklist

## 14. Development roadmap

A reasonable solo-development plan is approximately two to three weeks of part-time work, depending on how much time you spend on the terminal interface and concurrency testing.

| Milestone  | Deliverables                                         |
| ---------- | ---------------------------------------------------- |
| Day 1      | Go module, CLI skeleton, argument validation         |
| Days 2–3   | TCP scanner, timeouts, port parsing                  |
| Days 4–5   | Worker pool, context cancellation, concurrency tests |
| Days 6–7   | JSON reporting and readable terminal tables          |
| Days 8–9   | CIDR-based discovery and host inventory              |
| Days 10–11 | Service registry and live progress display           |
| Days 12–14 | Error handling, documentation, CI and release build  |
| Stretch    | Scan history, change detection, richer terminal UI   |

These milestones are planning estimates, not deadlines.

## 15. Stretch goals

Once the MVP works reliably, prioritize features that provide meaningful learning value.

[NetsCLI — Network Scanner for Desktop, CLI, and MCP](https://images.openai.com/static-rsc-4/FFA3o1k9AV7GH0QMdJD22ACNyj_EZXWoSgJM1bS77NjkfJmMysWpDEtwSm7ehmlT6gZP1ZAnFXv44rNrpTYpsKrShHSIJKgavnRH-_JD3RjViZnCdY-G66W1zMD0C1n3LndiA2ccBQQJFV9Pvle4P2ts7r61CYqUMgaveXeaaoQ?purpose=inline)

Network map

Visualize discovered hosts and their relationships using available subnet and gateway information. Clearly distinguish observed facts from inferred relationships.

[GitHub - yakimka/DiffLume: TUI app for viewing deltas between text files · GitHub](https://images.openai.com/static-rsc-4/HEsi4xTthZdQk1RqNmGtQsV7kQ6j8cQqXBi9gS6Kgvq45oKGz76dx4yBcDplP3ErDvFoJCiRrk7aGSqXDO57wHmt8QTgC8e1Ng6kXAOxvDP6ymgvQCr1qVDLDIqWogLKGREbMokmyr2293Zt9qdAyd-OhDhrlXtrA876K7Hl1y0?purpose=inline)

Change detection

Save scan snapshots and show which hosts and ports changed between scans.

[How to en”Rich” your Python console scripts | by Sourav De | Medium](https://images.openai.com/static-rsc-4/phfEAT08dFlSbNmThsYyZgtbFhFphuPSAlk0TCDL6uew8Kbb-fo604ERJdfbgsOv5Aq28oUWk4cmmXspg7Dkma1NrlNY9pgFuCdSC_5Vr_4QTHIiy0ykCRa9tPp3Or3wMOYbNDNk5rRWoAADDwnEhpfIpWyZjUs5_uvamDyoX-w?purpose=inline)

Interactive terminal dashboard

Use Bubble Tea to create navigable host tables, port details, scan controls, and filters.

[CLI Tool – DevTools](https://images.openai.com/static-rsc-4/T69WpZss9DX04xkPL5xzo2Qu5O0xpKrrbIN7Rs0smSTMrIqmWqkBc_GrUQeYO5rt4mgIQsfwvKCjtYR6pQMP9WwBMTy4drh1335yqRNzbAmgJFYNFNESVd1SX5NQVZkya7qYk-A1g_bu87wogtdoL9oZBxZI2eEgPaSbSVAVlPU?purpose=inline)

Automation mode

Add predictable JSON output, documented exit codes, and shell-friendly operation so the scanner can be integrated into scripts and local monitoring jobs.

## 16. Recommended first implementation

Start with Phase 1 only. Do not begin with the live dashboard, database, or advanced service detection.

Your first working version should support:

```
netrunner scan --target 127.0.0.1 --ports 1-1024
```

It should validate the arguments, create bounded scan jobs, connect to each TCP port with a timeout, collect results, print a summary, and optionally export JSON.

Use `127.0.0.1` and locally controlled test listeners during development. After the scanner works correctly, move to authorized subnet discovery.

That first version will already teach you Go modules, interfaces, network programming, goroutines, channels, context cancellation, testing, and CLI design. The subsequent phases turn it into a portfolio-quality systems project rather than just a port-scanning script.