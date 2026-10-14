// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Counter keeps a single row per product holding the available quantity.
// Every reservation for the same product updates that same row.
type Counter struct{}

// Name implements Strategy.
func (Counter) Name() string { return CounterStrategy }

// Reset implements Strategy.
func (Counter) Reset(ctx context.Context, db *sql.DB, productID int64, stock int) error {
	if _, err := db.ExecContext(ctx, "TRUNCATE inventory"); err != nil {
		return fmt.Errorf("truncating inventory: %w", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO inventory (product_id, available) VALUES ($1, $2)",
		productID, stock,
	); err != nil {
		return fmt.Errorf("inserting inventory: %w", err)
	}
	return prepareTable(ctx, db, "inventory")
}

// Reserve implements Strategy.
func (Counter) Reserve(ctx context.Context, db *sql.DB, productID int64, quantity int) error {
	var remaining int
	err := db.QueryRowContext(
		ctx,
		`
		UPDATE inventory
		SET available = available - $2
		WHERE product_id = $1
		  AND available >= $2
		RETURNING available
		`,
		productID,
		quantity,
	).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrOutOfStock
	}
	return err
}

// Available implements Strategy.
func (Counter) Available(ctx context.Context, db *sql.DB, productID int64) (int, error) {
	var available int
	err := db.QueryRowContext(ctx,
		"SELECT available FROM inventory WHERE product_id = $1",
		productID,
	).Scan(&available)
	return available, err
}
