CREATE EXTENSION IF NOT EXISTS btree_gist WITH SCHEMA public;

CREATE TABLE IF NOT EXISTS resources (
    id text PRIMARY KEY, name text NOT NULL, time_zone text NOT NULL,
    price_cents bigint NOT NULL CHECK (price_cents > 0),
    duration_minutes integer NOT NULL CHECK (duration_minutes > 0),
    buffer_minutes integer NOT NULL CHECK (buffer_minutes >= 0),
    slot_increment_minutes integer NOT NULL CHECK (slot_increment_minutes > 0),
    hold_seconds integer NOT NULL CHECK (hold_seconds > 0), created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS opening_windows (
    resource_id text NOT NULL REFERENCES resources(id), weekday smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
    opens_at time NOT NULL, closes_at time NOT NULL, PRIMARY KEY (resource_id, weekday), CHECK (opens_at < closes_at)
);
CREATE TABLE IF NOT EXISTS reservations (
    id uuid PRIMARY KEY, resource_id text NOT NULL REFERENCES resources(id), starts_at timestamptz NOT NULL,
    ends_at timestamptz NOT NULL, occupied_until timestamptz NOT NULL, amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    reservation_state text NOT NULL CHECK (reservation_state IN ('held','confirmed','completed','expired','cancelled')),
    payment_state text NOT NULL CHECK (payment_state IN ('pending','paid','expired','cancelled','payment_exception')),
    expires_at timestamptz NOT NULL, capability_hash bytea NOT NULL CHECK (octet_length(capability_hash) = 32),
    created_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
    CHECK (starts_at < ends_at AND ends_at <= occupied_until)
);
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'reservations_no_overlap' AND conrelid = 'reservations'::regclass) THEN
        ALTER TABLE reservations ADD CONSTRAINT reservations_no_overlap EXCLUDE USING gist
            (resource_id WITH =, tstzrange(starts_at, occupied_until, '[)') WITH &&)
            WHERE (reservation_state IN ('held','confirmed'));
    END IF;
END $$;
CREATE INDEX IF NOT EXISTS reservations_expiring_idx ON reservations (expires_at, id) WHERE reservation_state = 'held';
CREATE INDEX IF NOT EXISTS reservations_resource_start_idx ON reservations (resource_id, starts_at) WHERE reservation_state IN ('held','confirmed');
CREATE TABLE IF NOT EXISTS simulated_payments (
    id uuid PRIMARY KEY, reservation_id uuid NOT NULL UNIQUE REFERENCES reservations(id),
    amount_cents bigint NOT NULL CHECK (amount_cents > 0),
    payment_state text NOT NULL CHECK (payment_state IN ('pending','paid','expired','cancelled','payment_exception')),
    created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS payment_events (
    id uuid PRIMARY KEY, reservation_id uuid NOT NULL REFERENCES reservations(id), event_key text NOT NULL UNIQUE,
    event_type text NOT NULL CHECK (event_type = 'paid'), received_at timestamptz NOT NULL DEFAULT now(),
    processed_at timestamptz, lease_until timestamptz, attempts integer NOT NULL DEFAULT 0, last_error text
);
CREATE INDEX IF NOT EXISTS reservation_payment_events_pending_idx ON payment_events(received_at, id) WHERE processed_at IS NULL;
INSERT INTO resources (id,name,time_zone,price_cents,duration_minutes,buffer_minutes,slot_increment_minutes,hold_seconds)
VALUES ('demo-room','Sala de atendimento','America/Sao_Paulo',12000,30,15,15,300) ON CONFLICT (id) DO NOTHING;
INSERT INTO opening_windows (resource_id,weekday,opens_at,closes_at)
SELECT 'demo-room',day,'09:00','17:00' FROM generate_series(1,5) AS day ON CONFLICT (resource_id,weekday) DO NOTHING;
