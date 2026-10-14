// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

// Package inventory implements four ways of reserving inventory in PostgreSQL:
// decrementing a shared counter row; claiming individual reservation units with
// FOR UPDATE SKIP LOCKED, either from one large table or from a table split into
// buckets; and deleting units from a small pool replenished from the counter.
package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrOutOfStock is returned when the requested quantity could not be reserved.
var ErrOutOfStock = errors.New("out of stock")

// Strategy names.
const (
	CounterStrategy           = "counter"
	SkipLockedStrategy        = "skip-locked"
	ShardedSkipLockedStrategy = "skip-locked-sharded"
	PoolStrategy              = "skip-locked-pool"
)

// Strategy is a way of representing and reserving inventory.
type Strategy interface {
	// Name returns the strategy name, used as a metric label.
	Name() string

	// Reset recreates the dataset so the product has exactly stock units available.
	Reset(ctx context.Context, db *sql.DB, productID int64, stock int) error

	// Reserve reserves quantity units of the product, or none at all.
	Reserve(ctx context.Context, db *sql.DB, productID int64, quantity int) error

	// Available returns how many units of the product are still available.
	Available(ctx context.Context, db *sql.DB, productID int64) (int, error)
}

// Options holds the tunables of the strategies that have any.
type Options struct {
	// Buckets is the number of buckets for skip-locked-sharded.
	Buckets int
	// PoolSize is the maximum number of units in the pool for skip-locked-pool.
	PoolSize int
}

// DefaultOptions returns the options used by New.
func DefaultOptions() Options {
	return Options{Buckets: DefaultBuckets, PoolSize: DefaultPoolSize}
}

// New returns the strategy with the given name and default options.
func New(name string) (Strategy, error) {
	return NewWithOptions(name, DefaultOptions())
}

// NewWithOptions returns the strategy with the given name.
func NewWithOptions(name string, opts Options) (Strategy, error) {
	switch name {
	case CounterStrategy:
		return Counter{}, nil
	case SkipLockedStrategy:
		return SkipLocked{}, nil
	case ShardedSkipLockedStrategy:
		return ShardedSkipLocked{Buckets: opts.Buckets}, nil
	case PoolStrategy:
		return Pool{Size: opts.PoolSize}, nil
	}
	return nil, fmt.Errorf("unknown strategy %q", name)
}

// prepareTable leaves a freshly loaded table in a predictable physical state:
// statistics are up to date, the visibility map is set and the WAL produced by
// loading it has been flushed, so it does not leak into the measured run.
func prepareTable(ctx context.Context, db *sql.DB, table string) error {
	if _, err := db.ExecContext(ctx, "VACUUM ANALYZE "+table); err != nil {
		return fmt.Errorf("vacuuming %s: %w", table, err)
	}
	if _, err := db.ExecContext(ctx, "CHECKPOINT"); err != nil {
		return fmt.Errorf("checkpointing: %w", err)
	}
	return nil
}
