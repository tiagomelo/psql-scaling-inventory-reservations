// Copyright (c) 2026 Tiago Melo. All rights reserved.
// Use of this source code is governed by the MIT License that can be found in
// the LICENSE file.

package db

import (
	"context"
	"database/sql"
	_ "embed"
	"sync"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pkg/errors"
)

//go:embed schema.sql
var schema string

// Connect opens a connection pool to PostgreSQL and verifies it is reachable.
func Connect(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, errors.Wrap(err, "opening database")
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, errors.Wrap(err, "pinging database")
	}
	return db, nil
}

// Migrate creates the tables and indexes used by both strategies.
func Migrate(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return errors.Wrap(err, "applying schema")
	}
	return nil
}

// ResizePool sets the size of the Go connection pool and opens all of
// its connections upfront, so that establishing connections is not
// accounted as reservation latency during a benchmark run.
func ResizePool(ctx context.Context, db *sql.DB, size int) error {
	db.SetMaxOpenConns(size)
	db.SetMaxIdleConns(size)
	db.SetConnMaxIdleTime(0)
	db.SetConnMaxLifetime(0)

	conns := make([]*sql.Conn, size)
	errs := make([]error, size)
	var wg sync.WaitGroup
	for i := range size {
		wg.Go(func() {
			conns[i], errs[i] = db.Conn(ctx)
		})
	}
	wg.Wait()

	var firstErr error
	for i, conn := range conns {
		if conn != nil {
			conn.Close()
		}
		if errs[i] != nil && firstErr == nil {
			firstErr = errs[i]
		}
	}
	if firstErr != nil {
		return errors.Wrapf(firstErr, "warming up %d connections", size)
	}
	return nil
}
