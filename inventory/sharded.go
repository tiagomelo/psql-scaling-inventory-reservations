// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand/v2"

	"github.com/google/uuid"
)

// DefaultBuckets is the number of buckets the reservation pool is split into.
const DefaultBuckets = 64

// claimBucketUnits claims up to $4 available units of product $1 from bucket $2
// for reservation $3.
const claimBucketUnits = `
WITH candidate AS (
    SELECT id
    FROM bucketed_reservation_units
    WHERE product_id = $1
      AND bucket = $2
      AND reservation_id IS NULL
    ORDER BY id
    LIMIT $4
    FOR UPDATE SKIP LOCKED
)
UPDATE bucketed_reservation_units ru
SET reservation_id = $3,
    reserved_at = now()
FROM candidate
WHERE ru.id = candidate.id
RETURNING ru.id
`

// claimAnyUnits claims up to $3 available units of product $1, from any
// bucket, for reservation $2. It is the fallback for when the chosen bucket
// cannot satisfy the reservation on its own.
const claimAnyUnits = `
WITH candidate AS (
    SELECT id
    FROM bucketed_reservation_units
    WHERE product_id = $1
      AND reservation_id IS NULL
    ORDER BY bucket, id
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE bucketed_reservation_units ru
SET reservation_id = $2,
    reserved_at = now()
FROM candidate
WHERE ru.id = candidate.id
RETURNING ru.id
`

// ShardedSkipLocked is like SkipLocked, but the units of a product are split
// into buckets of contiguous rows. Each reservation starts in a random bucket,
// so concurrent reservations search different index and heap pages instead
// of all starting from the lowest available id.
type ShardedSkipLocked struct {
	Buckets int
}

// Name implements Strategy.
func (ShardedSkipLocked) Name() string { return ShardedSkipLockedStrategy }

// Reset implements Strategy.
func (s ShardedSkipLocked) Reset(ctx context.Context, db *sql.DB, productID int64, stock int) error {
	if _, err := db.ExecContext(ctx, "TRUNCATE bucketed_reservation_units RESTART IDENTITY"); err != nil {
		return fmt.Errorf("truncating bucketed_reservation_units: %w", err)
	}
	// Ordering by bucket gives each bucket a contiguous range of ids,
	// and therefore its own range of heap and index pages.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO bucketed_reservation_units (product_id, bucket)
		SELECT $1, g % $3::int
		FROM generate_series(1, $2::int) AS g
		ORDER BY g % $3::int, g`,
		productID, stock, s.Buckets,
	); err != nil {
		return fmt.Errorf("inserting reservation units: %w", err)
	}
	return prepareTable(ctx, db, "bucketed_reservation_units")
}

// Reserve implements Strategy.
func (s ShardedSkipLocked) Reserve(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	if quantity == 1 {
		return s.reserveOne(ctx, db, productID)
	}
	return s.reserveMany(ctx, db, productID, quantity)
}

// reserveOne claims a single unit from a random bucket, falling back to any
// bucket when the chosen one has no unit that can be claimed right now.
// Each statement claims at most one unit, so no explicit transaction is needed.
func (s ShardedSkipLocked) reserveOne(ctx context.Context, db *sql.DB, productID int64) error {
	reservationID := uuid.New()
	var unitID int64
	err := db.QueryRowContext(ctx, claimBucketUnits, productID, rand.IntN(s.Buckets), reservationID, 1).Scan(&unitID)
	if errors.Is(err, sql.ErrNoRows) {
		err = db.QueryRowContext(ctx, claimAnyUnits, productID, reservationID, 1).Scan(&unitID)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOutOfStock
	}
	return err
}

// reserveMany claims quantity units in a transaction, starting from a random
// bucket and topping up from any bucket. If fewer units than requested could
// be claimed, the transaction is rolled back.
func (s ShardedSkipLocked) reserveMany(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	reservationID := uuid.New()
	claimed, err := claim(ctx, tx, claimBucketUnits, productID, rand.IntN(s.Buckets), reservationID, quantity)
	if err != nil {
		return err
	}
	if claimed < quantity {
		more, err := claim(ctx, tx, claimAnyUnits, productID, reservationID, quantity-claimed)
		if err != nil {
			return err
		}
		claimed += more
	}
	if claimed < quantity {
		// Never commit a partial reservation.
		return ErrOutOfStock
	}
	return tx.Commit()
}

// Available implements Strategy.
func (ShardedSkipLocked) Available(ctx context.Context, db *sql.DB, productID int64) (int, error) {
	var available int
	err := db.QueryRowContext(ctx,
		`SELECT count(*)
		FROM bucketed_reservation_units
		WHERE product_id = $1
		  AND reservation_id IS NULL`,
		productID,
	).Scan(&available)
	return available, err
}
