package cli

import (
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestHelpAndVersion(t *testing.T) {
	if code, out, _ := run("--help"); code != 0 || !strings.Contains(out, "Examples") {
		t.Errorf("help: %d %q", code, out)
	}
	if code, out, _ := run("version"); code != 0 || !strings.Contains(out, Version) {
		t.Errorf("version: %d %q", code, out)
	}
	if code, _, _ := run("scan", "--help"); code != 0 {
		t.Errorf("scan --help exit %d", code)
	}
}

func TestInvalidArguments(t *testing.T) {
	cases := [][]string{
		{},
		{"bogus"},
		{"scan"},
		{"scan", "--target", "192.168.1.0/24"},
		{"scan", "--target", "127.0.0.1", "--ports", "70000"},
		{"scan", "--target", "127.0.0.1", "--ports", ""},
		{"scan", "--target", "127.0.0.1", "--workers", "0"},
		{"scan", "--target", "127.0.0.1", "--timeout", "-1s"},
		{"scan", "--target", "127.0.0.1", "--format", "xml"},
		{"scan", "--target", "127.0.0.1", "--nope"},
		{"scan", "--target", "127.0.0.1", "--output", filepath.Join("no-such-dir", "x.json")},
	}
	for _, args := range cases {
		if code, _, _ := run(args...); code != ExitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, ExitUsage)
		}
	}
}

func TestScanJSONToFile(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	port := strconv.Itoa(l.Addr().(*net.TCPAddr).Port)
	path := filepath.Join(t.TempDir(), "r.json")

	code, out, _ := run("scan", "--target", "127.0.0.1", "--ports", port, "--format", "json", "--output", path, "--quiet")
	if code != 0 || out != "" {
		t.Fatalf("exit %d stdout %q", code, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var rep struct {
		SchemaVersion int    `json:"schema_version"`
		Status        string `json:"status"`
		Summary       struct {
			Open int `json:"open"`
		} `json:"summary"`
	}
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if rep.SchemaVersion != 1 || rep.Status != "complete" || rep.Summary.Open != 1 {
		t.Errorf("report: %+v", rep)
	}
}

func TestScanJSONStdoutIsPure(t *testing.T) {
	code, out, _ := run("scan", "--target", "127.0.0.1", "--ports", "1", "--format", "json")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !json.Valid([]byte(out)) {
		t.Errorf("stdout is not pure JSON: %q", out)
	}
}
