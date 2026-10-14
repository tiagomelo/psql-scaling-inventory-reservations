# psql-scaling-inventory-reservations

A benchmark comparing four ways of reserving inventory in PostgreSQL under increasing concurrency, with Prometheus and Grafana to watch what the database is doing.

## Overview

All strategies perform the same logical operation:

> Reserve one unit of product 42 if inventory is available.

- **counter** — one row per product holding the available quantity. Every reservation runs
  `UPDATE inventory SET available = available - 1 ... RETURNING available` on the same row.
- **skip-locked** — one row per reservable unit. A reservation claims the first available unit
  with `FOR UPDATE SKIP LOCKED`.
- **skip-locked-sharded** — like `skip-locked`, but the units are split into 64 buckets of
  contiguous rows and each reservation starts searching in a random bucket.
- **skip-locked-pool** — inspired by [Shopify's MySQL design](https://shopify.engineering/scaling-inventory-reservations):
  a bounded pool (1,000 units by default) is refilled from the `inventory` counter, which remains the
  source of truth. A reservation `DELETE`s units from the pool with `SKIP LOCKED` and records them in
  `reserved_quantities`; when the pool runs dry, one transaction at a time refills it under an
  advisory lock.

After every run the benchmark checks that `initial stock - successful reservations == available units`, so no strategy can oversell without the run failing.

## Requirements

- [Go](https://go.dev/) 1.25+
- [Docker](https://www.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/)

## Stack

| Service | Image | Port |
|---------|-------|------|
| PostgreSQL 18 | `postgres:18` | 5432 |
| postgres_exporter | `quay.io/prometheuscommunity/postgres-exporter` | 9187 |
| Prometheus | `prom/prometheus` | 9090 |
| Grafana | `grafana/grafana` | 3000 |
| inventory-bench (host) | — | 2112 |

PostgreSQL runs with `max_connections=1100`, so 1000 workers can each hold a connection, and with
`init: true`, so the postmaster is not PID 1 (see Notes).

## Project Structure

```
psql-scaling-inventory-reservations/
├── .env
├── docker-compose.yaml
├── Makefile
├── bench/                  # worker pool, latency percentiles
├── cmd/inventory-bench/    # benchmark CLI
├── db/                     # connection, pool warm-up, schema.sql
├── grafana/
│   ├── dashboards/
│   │   ├── dashboards.yaml
│   │   └── inventory_benchmark.json
│   └── datasources/
│       └── datasources.yaml
├── inventory/              # counter, skip-locked, skip-locked-sharded and skip-locked-pool strategies
├── metrics/                # Prometheus metrics
├── postgres_exporter/
│   └── queries.yaml        # wait events, WAL position, table/index sizes, dead tuples
└── prometheus/
    └── prometheus.yml
```

## Getting Started

### 1. Run setup

```bash
make setup
```

If port 5432 is already taken on your machine, pick another one:

```bash
make setup POSTGRES_HOST_PORT=5434
```

(and pass the same `POSTGRES_HOST_PORT` to the other targets).

### 2. Open Grafana

Navigate to [http://localhost:3000](http://localhost:3000) and log in with `admin` / `admin123`.
The **Inventory reservations benchmark** dashboard is provisioned automatically.

### 3. Run the benchmark

```bash
make benchmark
```

It runs every strategy in `STRATEGIES` at every concurrency level in `WORKERS`, for `DURATION`
each (or for `OPERATIONS` reservation attempts each, when `DURATION` is empty). Before each run
the dataset is recreated (`TRUNCATE` + `INSERT`, then `VACUUM ANALYZE` and `CHECKPOINT`) and the Go
connection pool is resized to one connection per worker and warmed up. The results table is
printed at the end and written to `results/results.md`.

A single strategy or concurrency level:

```bash
make benchmark-counter WORKERS=100
make benchmark-skip-locked WORKERS=100
make benchmark STRATEGIES=skip-locked-sharded WORKERS=1,500
make benchmark STRATEGIES=skip-locked-pool WORKERS=1000
```

Strategy tunables are binary flags: `-buckets` (default 64) for `skip-locked-sharded` and
`-pool-size` (default 1000) for `skip-locked-pool`.

Or the binary directly:

```bash
make build
./bin/inventory-bench -dsn "$(make dsn)" -strategy=counter -workers=100 -operations=100000
./bin/inventory-bench -dsn "$(make dsn)" -strategy=skip-locked -workers=100 -duration=30s
```

### 4. Clean up

```bash
make clean
```

## Makefile Reference

```
make setup                  clean start of the whole stack + build
make benchmark              run every strategy at every concurrency level
make benchmark-counter      run only the counter strategy
make benchmark-skip-locked  run only the skip-locked strategy
make test                   unit tests + concurrency tests against Postgres
make psql-console           open a psql terminal inside the Postgres container
make clean                  remove all containers and volumes
```

## Configuration

Key variables in `.env`:

| Variable | Default | Description |
|----------|---------|-------------|
| `STRATEGIES` | `counter,skip-locked,skip-locked-sharded,skip-locked-pool` | strategies to run |
| `WORKERS` | `1,10,25,50,100,250,500,1000` | concurrency levels |
| `DURATION` | `30s` | how long each run lasts; empty to use `OPERATIONS` instead |
| `OPERATIONS` | `100000` | reservation attempts per run when `DURATION` is empty |
| `QUANTITY` | `1` | units per reservation |
| `STOCK` | `2000000` | units available at the start of each run |
| `COOLDOWN` | `20s` | pause between runs, so they are easy to tell apart in Grafana |
| `POSTGRES_MAX_CONNECTIONS` | `1100` | Postgres `max_connections` |
| `POSTGRES_HOST_PORT` | `5432` | Postgres port on the host |

## Results

`results/results.md` holds the table from the last `make benchmark`. The results discussed in the
article:

- `results/` — all four strategies, 30-second runs (the main results);
- `results/three-strategies/` — an earlier sweep of the first three strategies, 30-second runs;
- `results/100k-operations/` — the first three strategies, 100,000 reservations per run;
- `results/aborted-on-battery/` — a sweep interrupted by the laptop sleeping on battery, kept for reference only.

Panel screenshots are in `docs/grafana/`. Run benchmarks on AC power with sleep disabled
(e.g. `caffeinate -dims make benchmark` on macOS).

## Dashboard

**Application**: reservation throughput, concurrent workers, failed reservations, p50/p95/p99 latency.

**PostgreSQL**: transactions per second, backends waiting on locks, wait events, connections by state,
`pg_locks` by mode, WAL generated, dead tuples, table and index sizes, autovacuum runs.

**Go connection pool**: connections in use and time spent waiting for a connection.

## Notes

- `postgres_exporter` custom queries (`queries.yaml`) are deprecated upstream but still supported;
  they are the simplest way to export wait events and WAL position.
- The benchmark runs on the host; Prometheus scrapes it through `host.docker.internal:2112`.
- Without `init: true`, the postmaster is PID 1 in the container and adopts orphaned processes. Under
  heavy load a timed-out healthcheck leaves an orphaned `pg_isready` behind, and when it exits the
  postmaster treats it as a crashed child and restarts the server in crash recovery.
