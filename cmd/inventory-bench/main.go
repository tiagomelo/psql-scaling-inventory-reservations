// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/bench"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/db"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/inventory"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/metrics"
)

type options struct {
	dsn         string
	strategies  []string
	workers     []int
	operations  int
	duration    time.Duration
	quantity    int
	stock       int
	productID   int64
	strategy    inventory.Options
	cooldown    time.Duration
	metricsAddr string
	out         string
}

func parseOptions() (options, error) {
	var (
		opts       options
		strategies string
		workers    string
	)
	flag.StringVar(&opts.dsn, "dsn", os.Getenv("DSN"), "PostgreSQL connection string (defaults to $DSN)")
	flag.StringVar(&strategies, "strategy", "counter,skip-locked,skip-locked-sharded,skip-locked-pool", "comma-separated strategies: counter, skip-locked, skip-locked-sharded, skip-locked-pool")
	flag.StringVar(&workers, "workers", "100", "comma-separated concurrency levels, e.g. 1,10,100")
	flag.IntVar(&opts.operations, "operations", 100_000, "reservation attempts per run, unless -duration is set")
	flag.DurationVar(&opts.duration, "duration", 0, "how long each run lasts; overrides -operations")
	flag.IntVar(&opts.quantity, "quantity", 1, "units per reservation")
	flag.IntVar(&opts.stock, "stock", 1_000_000, "units available at the beginning of each run")
	flag.Int64Var(&opts.productID, "product", 42, "product to reserve")
	flag.IntVar(&opts.strategy.Buckets, "buckets", inventory.DefaultBuckets, "number of buckets for skip-locked-sharded")
	flag.IntVar(&opts.strategy.PoolSize, "pool-size", inventory.DefaultPoolSize, "maximum units in the pool for skip-locked-pool")
	flag.DurationVar(&opts.cooldown, "cooldown", 20*time.Second, "pause after each run, so runs are distinguishable in Grafana")
	flag.StringVar(&opts.metricsAddr, "metrics-addr", ":2112", "address to expose Prometheus metrics on")
	flag.StringVar(&opts.out, "out", "", "optional file to write the markdown results table to")
	flag.Parse()

	if opts.dsn == "" {
		return opts, errors.New("-dsn or $DSN is required")
	}
	for s := range strings.SplitSeq(strategies, ",") {
		s = strings.TrimSpace(s)
		if _, err := inventory.New(s); err != nil {
			return opts, err
		}
		opts.strategies = append(opts.strategies, s)
	}
	for w := range strings.SplitSeq(workers, ",") {
		n, err := strconv.Atoi(strings.TrimSpace(w))
		if err != nil || n < 1 {
			return opts, fmt.Errorf("invalid concurrency level %q", w)
		}
		opts.workers = append(opts.workers, n)
	}
	return opts, nil
}

func run(ctx context.Context, opts options, log *slog.Logger) error {
	// =========================================================================
	// Database support

	conn, err := db.Connect(ctx, opts.dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn); err != nil {
		return err
	}

	// =========================================================================
	// Metrics

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewDBStatsCollector(conn, "inventory"),
	)
	m := metrics.New(reg)

	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	srv := &http.Server{Addr: opts.metricsAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server", "error", err)
		}
	}()
	defer srv.Close()
	log.Info("exposing metrics", "addr", opts.metricsAddr)

	// =========================================================================
	// Benchmark

	var results []bench.Result
	for _, workers := range opts.workers {
		for _, name := range opts.strategies {
			s, _ := inventory.NewWithOptions(name, opts.strategy)
			res, err := runOnce(ctx, conn, s, workers, opts, m, log)
			if err != nil {
				return err
			}
			results = append(results, res)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(opts.cooldown):
			}
		}
	}

	printResults(os.Stdout, results)
	if opts.out != "" {
		f, err := os.Create(opts.out)
		if err != nil {
			return fmt.Errorf("creating %s: %w", opts.out, err)
		}
		defer f.Close()
		printResults(f, results)
	}
	return nil
}

// runOnce prepares a fresh dataset and connection pool and runs the benchmark
// once for the given strategy and concurrency level.
func runOnce(ctx context.Context, conn *sql.DB, s inventory.Strategy, workers int, opts options, m *metrics.Metrics, log *slog.Logger) (bench.Result, error) {
	log.Info("resetting dataset", "strategy", s.Name(), "stock", opts.stock)
	if err := s.Reset(ctx, conn, opts.productID, opts.stock); err != nil {
		return bench.Result{}, err
	}
	// One connection per worker, so the Go pool is not the bottleneck being measured.
	if err := db.ResizePool(ctx, conn, workers); err != nil {
		return bench.Result{}, err
	}

	log.Info("running", "strategy", s.Name(), "workers", workers, "operations", opts.operations, "duration", opts.duration)
	res := bench.Run(ctx, conn, s, bench.Config{
		ProductID:  opts.productID,
		Workers:    workers,
		Operations: opts.operations,
		Duration:   opts.duration,
		Quantity:   opts.quantity,
	}, m)

	available, err := s.Available(ctx, conn, opts.productID)
	if err != nil {
		return res, fmt.Errorf("checking available units: %w", err)
	}

	log.Info("done",
		"strategy", s.Name(),
		"workers", workers,
		"elapsed", res.Elapsed.Round(time.Millisecond),
		"reservations", res.Successes,
		"reservations/s", int(res.Throughput()),
		"p50", res.Latency.P50,
		"p95", res.Latency.P95,
		"p99", res.Latency.P99,
		"out_of_stock", res.OutOfStock,
		"errors", res.Errors,
		"available", available,
	)
	if res.FirstError != nil {
		log.Warn("reservations failed", "strategy", s.Name(), "errors", res.Errors, "first_error", res.FirstError)
	}

	expected := opts.stock - res.Successes*opts.quantity
	if available != expected {
		return res, fmt.Errorf("inconsistent inventory: %d units available, expected %d", available, expected)
	}
	return res, nil
}

func printResults(w io.Writer, results []bench.Result) {
	fmt.Fprintln(w, "| Workers | Strategy | Reservations | Reservations/s | p50 | p95 | p99 | max | Errors |")
	fmt.Fprintln(w, "|--------:|----------|-------------:|---------------:|----:|----:|----:|----:|-------:|")
	for _, r := range results {
		fmt.Fprintf(w, "| %d | %s | %d | %.0f | %s | %s | %s | %s | %d |\n",
			r.Workers,
			r.Strategy,
			r.Successes,
			r.Throughput(),
			formatDuration(r.Latency.P50),
			formatDuration(r.Latency.P95),
			formatDuration(r.Latency.P99),
			formatDuration(r.Latency.Max),
			r.Errors,
		)
	}
}

func formatDuration(d time.Duration) string {
	return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond))
}

func main() {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))
	opts, err := parseOptions()
	if err != nil {
		log.Error(err.Error())
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, opts, log); err != nil {
		log.Error(err.Error())
		os.Exit(1)
	}
}
