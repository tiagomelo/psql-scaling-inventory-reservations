// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package inventory

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
)

// DefaultPoolSize is the maximum number of claimable units kept in the pool
// for a product.
const DefaultPoolSize = 1000

// claimFromPool deletes up to $2 units of product $1 from the pool and
// records them as reservation $3. It returns how many units were claimed.
// When nothing could be claimed, no reservation is recorded.
const claimFromPool = `
WITH candidate AS (
    SELECT id
    FROM reservation_pool
    WHERE product_id = $1
    LIMIT $2
    FOR UPDATE SKIP LOCKED
),
claimed AS (
    DELETE FROM reservation_pool rp
    USING candidate
    WHERE rp.product_id = $1
      AND rp.id = candidate.id
    RETURNING rp.id
),
reserved AS (
    INSERT INTO reserved_quantities (reservation_id, product_id, quantity)
    SELECT $3, $1, count(*)
    FROM claimed
    HAVING count(*) > 0
    RETURNING quantity
)
SELECT coalesce((SELECT quantity FROM reserved), 0)
`

// Pool keeps a small, bounded pool of claimable units per product, in the
// spirit of Shopify's MySQL inventory reservations. The inventory table holds
// the units not yet moved into the pool. Reservations delete units from the
// pool with FOR UPDATE SKIP LOCKED and, when the pool runs dry, one
// transaction at a time replenishes it from the inventory table.
type Pool struct {
	Size int
}

// Name implements Strategy.
func (Pool) Name() string { return PoolStrategy }

// Reset implements Strategy.
func (p Pool) Reset(ctx context.Context, db *sql.DB, productID int64, stock int) error {
	if _, err := db.ExecContext(ctx,
		"TRUNCATE inventory, reservation_pool, reserved_quantities RESTART IDENTITY",
	); err != nil {
		return fmt.Errorf("truncating pool tables: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO inventory (product_id, available) VALUES ($1, $2)",
		productID, stock,
	); err != nil {
		return fmt.Errorf("inserting inventory: %w", err)
	}
	if _, err := p.replenish(ctx, db, productID); err != nil {
		return err
	}
	for _, table := range []string{"inventory", "reservation_pool", "reserved_quantities"} {
		if err := prepareTable(ctx, db, table); err != nil {
			return err
		}
	}
	return nil
}

// Reserve implements Strategy.
func (p Pool) Reserve(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	for {
		claimed, err := p.claim(ctx, db, productID, quantity)
		if err != nil {
			return err
		}
		if claimed == quantity {
			return nil
		}

		// The pool had nothing we could claim right now:
		// it's either empty or every unit in it is locked.
		poolSize, err := p.replenish(ctx, db, productID)
		if err != nil {
			return err
		}
		if poolSize < quantity {
			return ErrOutOfStock
		}
	}
}

// claim claims quantity units from the pool, or none at all.
func (Pool) claim(ctx context.Context, db *sql.DB, productID int64, quantity int) (int, error) {
	var claimed int
	if quantity == 1 {
		// At most one unit is claimed, so one autocommit statement is enough.
		err := db.QueryRowContext(ctx, claimFromPool, productID, 1, uuid.New()).Scan(&claimed)
		return claimed, err
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()
	if err := tx.QueryRowContext(ctx, claimFromPool, productID, quantity, uuid.New()).Scan(&claimed); err != nil {
		return 0, fmt.Errorf("claiming units: %w", err)
	}
	if claimed < quantity {
		// Never commit a partial reservation.
		return 0, nil
	}
	return claimed, tx.Commit()
}

// replenish tops the pool up to p.Size units, moving them out of the
// inventory table. Only one transaction per product replenishes at a time;
// concurrent callers wait for it and then find the pool already refilled.
// It returns the number of units in the pool afterwards.
func (p Pool) replenish(ctx context.Context, db *sql.DB, productID int64) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock($1)", productID); err != nil {
		return 0, fmt.Errorf("acquiring replenishment lock: %w", err)
	}

	// Counts units locked by in-flight reservations too: they are
	// either about to be reserved or about to be released.
	var inPool int
	if err := tx.QueryRowContext(ctx,
		"SELECT count(*) FROM reservation_pool WHERE product_id = $1",
		productID,
	).Scan(&inPool); err != nil {
		return 0, fmt.Errorf("counting pool: %w", err)
	}
	missing := p.Size - inPool
	if missing <= 0 {
		return inPool, tx.Commit()
	}

	var moved int
	if err := tx.QueryRowContext(ctx,
		`WITH current AS (
		    SELECT available
		    FROM inventory
		    WHERE product_id = $1
		    FOR UPDATE
		)
		UPDATE inventory i
		SET available = i.available - least(current.available, $2)
		FROM current
		WHERE i.product_id = $1
		RETURNING least(current.available, $2)`,
		productID, missing,
	).Scan(&moved); err != nil {
		return 0, fmt.Errorf("moving units out of inventory: %w", err)
	}
	if moved > 0 {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO reservation_pool (product_id)
			SELECT $1
			FROM generate_series(1, $2::int)`,
			productID, moved,
		); err != nil {
			return 0, fmt.Errorf("refilling pool: %w", err)
		}
	}
	return inPool + moved, tx.Commit()
}

// Available implements Strategy.
func (Pool) Available(ctx context.Context, db *sql.DB, productID int64) (int, error) {
	var available int
	err := db.QueryRowContext(ctx,
		`SELECT
		    (SELECT available FROM inventory WHERE product_id = $1) +
		    (SELECT count(*) FROM reservation_pool WHERE product_id = $1)`,
		productID,
	).Scan(&available)
	return available, err
}
