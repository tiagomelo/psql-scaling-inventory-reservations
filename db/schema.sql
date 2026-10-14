-- Strategy 1: one counter row per product.
CREATE TABLE IF NOT EXISTS inventory (
    product_id BIGINT PRIMARY KEY,
    available  INTEGER NOT NULL CHECK (available >= 0)
);

-- Strategy 2: one independently claimable row per reservable unit.
CREATE TABLE IF NOT EXISTS reservation_units (
    id             BIGSERIAL PRIMARY KEY,
    product_id     BIGINT NOT NULL,
    reservation_id UUID,
    reserved_at    TIMESTAMPTZ
);

-- Only available units are indexed. Reserved units leave the index
-- once vacuum removes the entries pointing to their old row versions.
--
-- In this benchmark every unit belongs to the same product, so a
-- (product_id, id) index would filter nothing while being larger than the
-- primary key, and the planner would rather walk the primary key, filtering
-- out every already reserved unit. With many products, (product_id, id) is
-- the natural choice. Either way, check the plan with EXPLAIN.
CREATE INDEX IF NOT EXISTS idx_reservation_units_available
ON reservation_units (id)
WHERE reservation_id IS NULL;

-- Strategy 3: same as strategy 2, but units are split into buckets
-- of contiguous ids, so concurrent reservations can start searching
-- in different parts of the table and of the index.
CREATE TABLE IF NOT EXISTS bucketed_reservation_units (
    id             BIGSERIAL PRIMARY KEY,
    product_id     BIGINT NOT NULL,
    bucket         INTEGER NOT NULL,
    reservation_id UUID,
    reserved_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_bucketed_reservation_units_available
ON bucketed_reservation_units (product_id, bucket, id)
WHERE reservation_id IS NULL;

-- Strategy 4: a small, bounded pool of claimable units per product,
-- replenished from the inventory table, which remains the source of truth.
-- Reserving a unit deletes it from the pool and records the reservation.
CREATE TABLE IF NOT EXISTS reservation_pool (
    product_id BIGINT NOT NULL,
    id         BIGSERIAL,
    PRIMARY KEY (product_id, id)
);

CREATE TABLE IF NOT EXISTS reserved_quantities (
    reservation_id UUID PRIMARY KEY,
    product_id     BIGINT NOT NULL,
    quantity       INTEGER NOT NULL,
    reserved_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
