package cli

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/kxngHADES/NetRunner/internal/scanner"
)

func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func useColor(w io.Writer) bool {
	return isTerminal(w) && os.Getenv("NO_COLOR") == ""
}

// progress renders a single self-updating line on stderr, only when stderr is
// an interactive terminal. When redirected it stays silent, so logs are not
// flooded; the final report carries the totals.
type progress struct {
	w       io.Writer
	enabled bool
	start   time.Time
	last    time.Time
	shown   bool
}

func newProgress(w io.Writer, enabled bool) *progress {
	return &progress{w: w, enabled: enabled && isTerminal(w), start: time.Now()}
}

func (p *progress) update(s scanner.Progress) {
	if !p.enabled {
		return
	}
	now := time.Now()
	if now.Sub(p.last) < 100*time.Millisecond && s.Done != s.Total {
		return
	}
	p.last = now
	pct := 0
	if s.Total > 0 {
		pct = s.Done * 100 / s.Total
	}
	const width = 24
	filled := pct * width / 100
	bar := strings.Repeat("#", filled) + strings.Repeat(".", width-filled)
	fmt.Fprintf(p.w, "\r[%s] %3d%%  %d/%d  open: %d  workers: %d  %s   ",
		bar, pct, s.Done, s.Total, s.Open, s.Active, now.Sub(p.start).Round(time.Second))
	p.shown = true
}

func (p *progress) finish() {
	if p.shown {
		fmt.Fprint(p.w, "\r"+strings.Repeat(" ", 90)+"\r")
	}
}
