package scanner

import (
	"context"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Limits for the worker pool.
const (
	DefaultWorkers = 100
	MaxWorkers     = 1000
	DefaultTimeout = time.Second
	MaxTimeout     = time.Minute
)

// Progress is a snapshot passed to the progress callback.
type Progress struct {
	Done   int
	Total  int
	Open   int
	Active int
	Result PortResult // the result that triggered this update
}

// Outcome is the raw result of a scan run.
type Outcome struct {
	Results    []PortResult
	Total      int
	Incomplete bool // true if cancellation stopped the scan before all jobs ran
}

type job struct {
	host string
	port uint16
}

// Scan runs check against every host×port pair using a bounded worker pool.
// It returns when all workers have exited. onProgress (optional) is called
// from the calling goroutine after every completed job.
func Scan(ctx context.Context, cfg Config, check Checker, onProgress func(Progress)) Outcome {
	total := len(cfg.Targets) * len(cfg.Ports)
	jobs := make(chan job, cfg.Workers)
	results := make(chan PortResult, cfg.Workers)

	var active atomic.Int32
	var wg sync.WaitGroup

	// Producer: stops scheduling as soon as the context is cancelled.
	go func() {
		defer close(jobs)
		for _, h := range cfg.Targets {
			for _, p := range cfg.Ports {
				select {
				case jobs <- job{h, p}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	for i := 0; i < cfg.Workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				if ctx.Err() != nil {
					continue // drain remaining buffered jobs without work
				}
				active.Add(1)
				r := check(ctx, j.host, j.port, cfg.Timeout)
				active.Add(-1)
				if r.State == stateCancelled {
					continue
				}
				results <- r
			}
		}()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	out := Outcome{Total: total, Results: make([]PortResult, 0, total)}
	open := 0
	for r := range results {
		out.Results = append(out.Results, r)
		if r.State == StateOpen {
			open++
		}
		if onProgress != nil {
			onProgress(Progress{Done: len(out.Results), Total: total, Open: open, Active: int(active.Load()), Result: r})
		}
	}
	out.Incomplete = len(out.Results) < total

	order := make(map[string]int, len(cfg.Targets))
	for i, h := range cfg.Targets {
		order[h] = i
	}
	sort.SliceStable(out.Results, func(i, j int) bool {
		a, b := out.Results[i], out.Results[j]
		if a.Host != b.Host {
			return order[a.Host] < order[b.Host]
		}
		return a.Port < b.Port
	})
	return out
}
