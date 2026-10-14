// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package inventory

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tiagomelo/psql-scaling-inventory-reservations/db"
)

const productID = 42

// connect returns a connection to the database pointed to by TEST_DSN,
// skipping the test when it is not set.
func connect(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DSN")
	if dsn == "" {
		t.Skip("TEST_DSN not set")
	}
	ctx := context.Background()
	conn, err := db.Connect(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	require.NoError(t, db.Migrate(ctx, conn))
	require.NoError(t, db.ResizePool(ctx, conn, 50))
	return conn
}

func TestReserveNeverOversells(t *testing.T) {
	conn := connect(t)
	ctx := context.Background()

	testCases := []struct {
		name               string
		strategy           Strategy
		stock              int
		quantity           int
		attempts           int
		expectedSuccesses  int64
		expectedOutOfStock int64
		expectedAvailable  int
	}{
		{
			name:               "counter, one unit per reservation",
			strategy:           Counter{},
			stock:              100,
			quantity:           1,
			attempts:           300,
			expectedSuccesses:  100,
			expectedOutOfStock: 200,
			expectedAvailable:  0,
		},
		{
			name:               "counter, three units per reservation",
			strategy:           Counter{},
			stock:              100,
			quantity:           3,
			attempts:           300,
			expectedSuccesses:  33,
			expectedOutOfStock: 267,
			expectedAvailable:  1,
		},
		{
			name:               "skip locked, one unit per reservation",
			strategy:           SkipLocked{},
			stock:              100,
			quantity:           1,
			attempts:           300,
			expectedSuccesses:  100,
			expectedOutOfStock: 200,
			expectedAvailable:  0,
		},
		{
			name:               "sharded skip locked, one unit per reservation",
			strategy:           ShardedSkipLocked{Buckets: 8},
			stock:              100,
			quantity:           1,
			attempts:           300,
			expectedSuccesses:  100,
			expectedOutOfStock: 200,
			expectedAvailable:  0,
		},
		{
			name:               "pool, one unit per reservation, replenished several times",
			strategy:           Pool{Size: 10},
			stock:              100,
			quantity:           1,
			attempts:           300,
			expectedSuccesses:  100,
			expectedOutOfStock: 200,
			expectedAvailable:  0,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.strategy.Reset(ctx, conn, productID, tc.stock))

			var successes, outOfStock atomic.Int64
			var wg sync.WaitGroup
			for range tc.attempts {
				wg.Go(func() {
					err := tc.strategy.Reserve(ctx, conn, productID, tc.quantity)
					switch {
					case err == nil:
						successes.Add(1)
					case errors.Is(err, ErrOutOfStock):
						outOfStock.Add(1)
					default:
						t.Errorf("unexpected error: %v", err)
					}
				})
			}
			wg.Wait()

			require.Equal(t, tc.expectedSuccesses, successes.Load())
			require.Equal(t, tc.expectedOutOfStock, outOfStock.Load())
			available, err := tc.strategy.Available(ctx, conn, productID)
			require.NoError(t, err)
			require.Equal(t, tc.expectedAvailable, available)
		})
	}
}

// When a multi-unit reservation fails it rolls back the units it had claimed.
// A concurrent reservation may have skipped those units while they were locked,
// so with SKIP LOCKED the exact number of successes is not deterministic.
// What must always hold is that every reserved unit is accounted for.
func TestShardedSkipLockedMultiUnitReservationsAreConsistent(t *testing.T) {
	conn := connect(t)
	ctx := context.Background()
	s := ShardedSkipLocked{Buckets: 8}
	const (
		stock    = 100
		quantity = 3
		attempts = 300
	)

	require.NoError(t, s.Reset(ctx, conn, productID, stock))

	var successes atomic.Int64
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			err := s.Reserve(ctx, conn, productID, quantity)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ErrOutOfStock):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()

	available, err := s.Available(ctx, conn, productID)
	require.NoError(t, err)
	require.Equal(t, stock, int(successes.Load())*quantity+available)
	require.LessOrEqual(t, successes.Load(), int64(stock/quantity))
}

func TestPoolMultiUnitReservationsAreConsistent(t *testing.T) {
	conn := connect(t)
	ctx := context.Background()
	s := Pool{Size: 10}
	const (
		stock    = 100
		quantity = 3
		attempts = 300
	)

	require.NoError(t, s.Reset(ctx, conn, productID, stock))

	var successes atomic.Int64
	var wg sync.WaitGroup
	for range attempts {
		wg.Go(func() {
			err := s.Reserve(ctx, conn, productID, quantity)
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ErrOutOfStock):
			default:
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
	wg.Wait()

	available, err := s.Available(ctx, conn, productID)
	require.NoError(t, err)
	require.Equal(t, stock, int(successes.Load())*quantity+available)

	var reserved int
	require.NoError(t, conn.QueryRowContext(ctx,
		"SELECT coalesce(sum(quantity), 0) FROM reserved_quantities WHERE product_id = $1",
		productID,
	).Scan(&reserved))
	require.Equal(t, int(successes.Load())*quantity, reserved)
}

func TestSkipLockedNeverCommitsPartialReservations(t *testing.T) {
	conn := connect(t)
	ctx := context.Background()
	s := SkipLocked{}

	require.NoError(t, s.Reset(ctx, conn, productID, 4))

	// Only four units exist, so asking for five must fail without claiming any.
	err := s.Reserve(ctx, conn, productID, 5)
	require.ErrorIs(t, err, ErrOutOfStock)

	available, err := s.Available(ctx, conn, productID)
	require.NoError(t, err)
	require.Equal(t, 4, available)

	require.NoError(t, s.Reserve(ctx, conn, productID, 4))
	available, err = s.Available(ctx, conn, productID)
	require.NoError(t, err)
	require.Equal(t, 0, available)
}
