// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

// Package bench runs concurrent inventory reservations against a strategy
// and summarizes throughput and latency.
package bench

import (
	"context"
	"database/sql"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tiagomelo/psql-scaling-inventory-reservations/inventory"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/metrics"
)

// Config describes one benchmark run. The run stops after Duration
// when it is set, or after Operations reservation attempts otherwise.
type Config struct {
	ProductID  int64
	Workers    int
	Operations int
	Duration   time.Duration
	Quantity   int
}

// Result summarizes one benchmark run.
type Result struct {
	Strategy   string
	Workers    int
	Operations int
	Successes  int
	OutOfStock int
	Errors     int
	Elapsed    time.Duration
	Latency    Latency
	// FirstError is the first unexpected error seen in the run, if any.
	FirstError error
}

// Throughput returns successful reservations per second.
func (r Result) Throughput() float64 {
	return float64(r.Successes) / r.Elapsed.Seconds()
}

// workerResult holds what a single worker observed.
type workerResult struct {
	latencies  []time.Duration
	outOfStock int
	errors     int
	firstError error
}

// Run executes reservations spread across cfg.Workers goroutines
// and returns a summary of the run.
func Run(ctx context.Context, db *sql.DB, s inventory.Strategy, cfg Config, m *metrics.Metrics) Result {
	strategy := s.Name()
	duration := m.ReservationDuration.WithLabelValues(strategy)
	success := m.Reservations.WithLabelValues(strategy, "success")
	outOfStock := m.Reservations.WithLabelValues(strategy, "out_of_stock")
	failure := m.Reservations.WithLabelValues(strategy, "error")

	reserve := func(ctx context.Context, db *sql.DB) error {
		return s.Reserve(ctx, db, cfg.ProductID, cfg.Quantity)
	}

	// next tells a worker whether it should attempt another reservation.
	var next func() bool
	if cfg.Duration > 0 {
		deadline := time.Now().Add(cfg.Duration)
		next = func() bool { return time.Now().Before(deadline) }
	} else {
		var remaining atomic.Int64
		remaining.Store(int64(cfg.Operations))
		next = func() bool { return remaining.Add(-1) >= 0 }
	}

	m.Workers.WithLabelValues(strategy).Set(float64(cfg.Workers))
	defer m.Workers.WithLabelValues(strategy).Set(0)

	results := make([]workerResult, cfg.Workers)
	var wg sync.WaitGroup
	started := time.Now()
	for i := range cfg.Workers {
		wg.Go(func() {
			results[i] = runWorker(ctx, db, next, reserve, func(elapsed time.Duration, err error) {
				duration.Observe(elapsed.Seconds())
				switch {
				case err == nil:
					success.Inc()
				case errors.Is(err, inventory.ErrOutOfStock):
					outOfStock.Inc()
				default:
					failure.Inc()
				}
			})
		})
	}
	wg.Wait()
	elapsed := time.Since(started)

	res := Result{
		Strategy: strategy,
		Workers:  cfg.Workers,
		Elapsed:  elapsed,
	}
	var latencies []time.Duration
	for _, r := range results {
		latencies = append(latencies, r.latencies...)
		res.OutOfStock += r.outOfStock
		res.Errors += r.errors
		if res.FirstError == nil {
			res.FirstError = r.firstError
		}
	}
	res.Successes = len(latencies)
	res.Operations = res.Successes + res.OutOfStock + res.Errors
	res.Latency = Summarize(latencies)
	return res
}

// runWorker reserves inventory until next returns false or ctx is done.
// Latencies are only recorded for successful reservations.
func runWorker(
	ctx context.Context,
	db *sql.DB,
	next func() bool,
	reserve func(context.Context, *sql.DB) error,
	observe func(time.Duration, error),
) workerResult {
	var res workerResult
	for next() {
		if ctx.Err() != nil {
			return res
		}

		started := time.Now()
		err := reserve(ctx, db)
		elapsed := time.Since(started)
		observe(elapsed, err)

		switch {
		case err == nil:
			res.latencies = append(res.latencies, elapsed)
		case errors.Is(err, inventory.ErrOutOfStock):
			res.outOfStock++
		default:
			res.errors++
			if res.firstError == nil {
				res.firstError = err
			}
		}
	}
	return res
}
