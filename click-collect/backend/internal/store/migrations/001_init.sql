CREATE TABLE products (
    id text PRIMARY KEY,
    title text NOT NULL CHECK (btrim(title) <> ''),
    description text NOT NULL,
    price_cents bigint NOT NULL CHECK (price_cents > 0),
    total_units integer NOT NULL CHECK (total_units >= 0),
    available_units integer NOT NULL CHECK (available_units >= 0 AND available_units <= total_units),
    reservation_ttl_seconds integer NOT NULL CHECK (reservation_ttl_seconds > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id text PRIMARY KEY,
    product_id text NOT NULL REFERENCES products(id),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    payment_state text NOT NULL CHECK (payment_state IN ('pending', 'paid', 'expired', 'cancelled', 'payment_exception')),
    fulfillment_state text NOT NULL CHECK (fulfillment_state IN ('awaiting_payment', 'preparing', 'ready_for_pickup', 'picked_up')),
    expires_at timestamptz NOT NULL,
    access_token_hash text NOT NULL CHECK (access_token_hash ~ '^[0-9a-f]{64}$'),
    access_expires_at timestamptz NOT NULL,
    pickup_code_hash text NOT NULL CHECK (pickup_code_hash ~ '^[0-9a-f]{64}$'),
    pickup_consumed_at timestamptz,
    picked_up_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK ((fulfillment_state = 'picked_up') = (picked_up_at IS NOT NULL)),
    CHECK ((fulfillment_state = 'picked_up') = (pickup_consumed_at IS NOT NULL))
);

CREATE INDEX orders_expiring_idx ON orders (expires_at) WHERE payment_state = 'pending';
CREATE INDEX orders_operator_queue_idx ON orders (created_at DESC) WHERE payment_state = 'paid';

CREATE TABLE payment_events (
    id bigserial PRIMARY KEY,
    order_id text NOT NULL REFERENCES orders(id),
    event_key text NOT NULL UNIQUE,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error text
);

CREATE INDEX payment_events_pending_idx ON payment_events (received_at, id) WHERE processed_at IS NULL;

CREATE TABLE simulated_charges (
    order_id text PRIMARY KEY REFERENCES orders(id),
    charge_id text NOT NULL UNIQUE,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    status text NOT NULL CHECK (status IN ('pending', 'paid')),
    event_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

INSERT INTO products (id, title, description, price_cents, total_units, available_units, reservation_ttl_seconds)
VALUES ('house-cake', 'Bolo de fubá da casa', 'Fatia generosa, preparada hoje na loja.', 890, 12, 12, 180)
ON CONFLICT (id) DO NOTHING;
