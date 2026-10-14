// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
)

// claimUnits claims up to $3 available units of product $1 for reservation $2.
// Units locked by concurrent transactions are skipped instead of waited for.
const claimUnits = `
WITH candidate AS (
    SELECT id
    FROM reservation_units
    WHERE product_id = $1
      AND reservation_id IS NULL
    ORDER BY id
    LIMIT $3
    FOR UPDATE SKIP LOCKED
)
UPDATE reservation_units ru
SET reservation_id = $2,
    reserved_at = now()
FROM candidate
WHERE ru.id = candidate.id
RETURNING ru.id
`

// SkipLocked keeps one row per reservable unit. Concurrent reservations
// claim different rows using FOR UPDATE SKIP LOCKED.
type SkipLocked struct{}

// Name implements Strategy.
func (SkipLocked) Name() string { return SkipLockedStrategy }

// Reset implements Strategy.
func (SkipLocked) Reset(ctx context.Context, db *sql.DB, productID int64, stock int) error {
	// Recreating the dataset instead of updating every row back to NULL
	// avoids starting each run with a million dead tuples.
	if _, err := db.ExecContext(ctx, "TRUNCATE reservation_units RESTART IDENTITY"); err != nil {
		return fmt.Errorf("truncating reservation_units: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO reservation_units (product_id)
		SELECT $1
		FROM generate_series(1, $2::int)`,
		productID, stock,
	); err != nil {
		return fmt.Errorf("inserting reservation units: %w", err)
	}
	return prepareTable(ctx, db, "reservation_units")
}

// Reserve implements Strategy.
func (s SkipLocked) Reserve(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	if quantity == 1 {
		return s.reserveOne(ctx, db, productID)
	}
	return s.reserveMany(ctx, db, productID, quantity)
}

// reserveOne claims a single unit in one autocommit statement.
func (SkipLocked) reserveOne(ctx context.Context, db *sql.DB, productID int64) error {
	var unitID int64
	err := db.QueryRowContext(ctx, claimUnits, productID, uuid.New(), 1).Scan(&unitID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOutOfStock
	}
	return err
}

// reserveMany claims quantity units in a transaction. If fewer units than
// requested could be claimed, the transaction is rolled back so that a
// partial reservation is never committed.
func (SkipLocked) reserveMany(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	claimed, err := claim(ctx, tx, claimUnits, productID, uuid.New(), quantity)
	if err != nil {
		return err
	}
	if claimed < quantity {
		// Never commit a partial reservation.
		return ErrOutOfStock
	}
	return tx.Commit()
}

// claim runs a claiming query and returns how many units it claimed.
func claim(ctx context.Context, tx *sql.Tx, query string, args ...any) (int, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("claiming units: %w", err)
	}
	defer rows.Close()
	claimed := 0
	for rows.Next() {
		claimed++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading claimed units: %w", err)
	}
	return claimed, nil
}

// Available implements Strategy.
func (SkipLocked) Available(ctx context.Context, db *sql.DB, productID int64) (int, error) {
	var available int
	err := db.QueryRowContext(ctx,
		`SELECT count(*)
		FROM reservation_units
		WHERE product_id = $1
		  AND reservation_id IS NULL`,
		productID,
	).Scan(&available)
	return available, err
}
