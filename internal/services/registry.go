// Package services maps TCP port numbers to conventional service names.
// Mappings are hints, not proof: a service can run on any port.
package services

var registry = map[uint16]string{
	21:   "ftp",
	22:   "ssh",
	23:   "telnet",
	25:   "smtp",
	53:   "dns",
	80:   "http",
	110:  "pop3",
	135:  "msrpc",
	139:  "netbios-ssn",
	143:  "imap",
	443:  "https",
	445:  "smb",
	993:  "imaps",
	995:  "pop3s",
	1433: "mssql",
	3306: "mysql",
	3389: "rdp",
	5432: "postgresql",
	5900: "vnc",
	6379: "redis",
	8080: "http-alt",
	8443: "https-alt",
}

// Lookup returns the conventional service for a port, or "" if unknown.
func Lookup(port uint16) string {
	return registry[port]
}
