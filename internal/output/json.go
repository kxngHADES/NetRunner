// Package output formats scan reports. It performs no network operations.
package output

import (
	"encoding/json"
	"io"
)

// WriteJSON writes a report (scan or discovery) as indented JSON.
func WriteJSON(w io.Writer, rep any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(rep)
}
