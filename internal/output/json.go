// Package output formats scan reports. It performs no network operations.
package output

import (
	"encoding/json"
	"io"

	"github.com/kxngHADES/NetRunner/internal/scanner"
)

// WriteJSON writes the report as indented JSON.
func WriteJSON(w io.Writer, rep scanner.ScanReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
