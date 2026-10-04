CREATE TABLE offers (
    id text PRIMARY KEY,
    title text NOT NULL,
    price_cents bigint NOT NULL CHECK (price_cents > 0),
    total_units integer NOT NULL CHECK (total_units >= 0),
    available_units integer NOT NULL CHECK (available_units >= 0 AND available_units <= total_units),
    reservation_ttl_seconds integer NOT NULL CHECK (reservation_ttl_seconds > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE orders (
    id text PRIMARY KEY,
    offer_id text NOT NULL REFERENCES offers(id),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    state text NOT NULL CHECK (state IN ('pending_payment', 'paid', 'expired', 'payment_exception')),
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX orders_expiring_idx ON orders (expires_at) WHERE state = 'pending_payment';

CREATE TABLE payment_events (
    id text PRIMARY KEY,
    order_id text NOT NULL REFERENCES orders(id),
    event_type text NOT NULL CHECK (event_type = 'paid'),
    event_key text NOT NULL UNIQUE,
    received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz,
    lease_until timestamptz,
    attempts integer NOT NULL DEFAULT 0,
    last_error text
);

CREATE INDEX payment_events_pending_idx ON payment_events(received_at) WHERE processed_at IS NULL;

CREATE TABLE simulated_charges (
    order_id text PRIMARY KEY REFERENCES orders(id),
    charge_id text NOT NULL UNIQUE,
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    status text NOT NULL CHECK (status IN ('pending', 'paid')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
