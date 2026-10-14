.DEFAULT_GOAL := help

include .env
export

# PSQL domain source name string
PSQL_DSN ?= postgres://$(POSTGRES_USER):$(POSTGRES_PASSWORD)@$(POSTGRES_HOST):$(POSTGRES_HOST_PORT)/$(POSTGRES_DB)?sslmode=disable

.PHONY: help
## help: shows this help message
help:
	@ echo "Usage: make [target]\n"
	@ sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

# ==============================================================================
# DB

.PHONY: dsn
## dsn: shows the psql dsn string
dsn:
	@ echo $(PSQL_DSN)

.PHONY: start-psql
## start-psql: starts postgres instance
start-psql:
	@ docker-compose up -d $(POSTGRES_DATABASE_CONTAINER_NAME)
	@ echo "Waiting for Postgres to start..."
	@ until docker exec $(POSTGRES_DATABASE_CONTAINER_NAME) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB) -c "SELECT 1;" >/dev/null 2>&1; do \
		echo "Postgres not ready, sleeping for 5 seconds..."; \
		sleep 5; \
	done
	@ echo "Postgres is up and running."

.PHONY: stop-psql
## stop-psql: stops postgres instance
stop-psql:
	@ docker-compose down $(POSTGRES_DATABASE_CONTAINER_NAME)

.PHONY: psql-console
## psql-console: opens psql terminal
psql-console: start-psql
	@ docker exec -it $(POSTGRES_DATABASE_CONTAINER_NAME) psql -U $(POSTGRES_USER) -d $(POSTGRES_DB)

# ==============================================================================
# Monitoring

.PHONY: start-monitoring
## start-monitoring: starts monitoring stack (Prometheus, Grafana, postgres_exporter)
start-monitoring: start-psql
	@ docker-compose up -d \
		$(PROMETHEUS_CONTAINER_NAME) \
		$(GRAFANA_CONTAINER_NAME) \
		$(GRAFANA_RENDERER_CONTAINER_NAME) \
		$(POSTGRES_EXPORTER_CONTAINER_NAME)
	@ echo "Monitoring stack is up."
	@ echo "Grafana:    http://localhost:3000"
	@ echo "Prometheus: http://localhost:$(PROMETHEUS_PORT)"

.PHONY: stop-monitoring
## stop-monitoring: stops monitoring stack
stop-monitoring:
	@ docker-compose down -v \
		$(PROMETHEUS_CONTAINER_NAME) \
		$(GRAFANA_CONTAINER_NAME) \
		$(GRAFANA_RENDERER_CONTAINER_NAME) \
		$(POSTGRES_EXPORTER_CONTAINER_NAME)

# ==============================================================================
# benchmark

.PHONY: build
## build: builds the benchmark binary
build:
	@ go build -o bin/inventory-bench ./cmd/inventory-bench

.PHONY: setup
## setup: full setup - cleans and starts the whole stack (run once before benchmarks)
setup: clean start-monitoring build

.PHONY: benchmark
## benchmark: runs both strategies at every concurrency level in WORKERS
benchmark: build
	@ mkdir -p results
	@ ./bin/inventory-bench \
		-dsn "$(PSQL_DSN)" \
		-strategy=$(STRATEGIES) \
		-workers=$(WORKERS) \
		-operations=$(OPERATIONS) \
		-duration=$(or $(DURATION),0) \
		-quantity=$(QUANTITY) \
		-stock=$(STOCK) \
		-cooldown=$(COOLDOWN) \
		-metrics-addr=:$(METRICS_PORT) \
		-out=results/results.md

.PHONY: benchmark-counter
## benchmark-counter: runs only the counter strategy, e.g. make benchmark-counter WORKERS=100
benchmark-counter: STRATEGIES=counter
benchmark-counter: benchmark

.PHONY: benchmark-skip-locked
## benchmark-skip-locked: runs only the skip-locked strategy, e.g. make benchmark-skip-locked WORKERS=100
benchmark-skip-locked: STRATEGIES=skip-locked
benchmark-skip-locked: benchmark

# ==============================================================================
# tests

.PHONY: test
## test: runs unit tests and the concurrency tests against postgres
test: start-psql
	@ TEST_DSN="$(PSQL_DSN)" go test -v ./... -count=1

# ==============================================================================
# cleaning

.PHONY: clean
## clean: removes all containers, volumes and binaries
clean:
	@ docker-compose down -v
	@ rm -rf bin
