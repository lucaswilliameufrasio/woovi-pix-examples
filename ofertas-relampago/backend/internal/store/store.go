package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrUnavailable = errors.New("offer unavailable")
var ErrEventKeyConflict = errors.New("event key already belongs to another order")
var ErrChargeConflict = errors.New("charge order already has a different amount")

type Offer struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	PriceCents           int64  `json:"price_cents"`
	AvailableUnits       int    `json:"available_units"`
	ReservationTTLSecond int    `json:"reservation_ttl_seconds"`
}

type Order struct {
	ID        string    `json:"id"`
	OfferID   string    `json:"offer_id"`
	Amount    int64     `json:"amount_cents"`
	State     string    `json:"state"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SimulatedCharge struct {
	ID          string `json:"id"`
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Status      string `json:"status"`
}

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	const schema = `CREATE TABLE IF NOT EXISTS offers (
		id text PRIMARY KEY, title text NOT NULL, price_cents bigint NOT NULL CHECK (price_cents > 0),
		total_units integer NOT NULL CHECK (total_units >= 0), available_units integer NOT NULL CHECK (available_units >= 0 AND available_units <= total_units),
		reservation_ttl_seconds integer NOT NULL CHECK (reservation_ttl_seconds > 0), created_at timestamptz NOT NULL DEFAULT now());
		CREATE TABLE IF NOT EXISTS orders (
		id text PRIMARY KEY, offer_id text NOT NULL REFERENCES offers(id), amount_cents bigint NOT NULL CHECK (amount_cents > 0),
		state text NOT NULL CHECK (state IN ('pending_payment','paid','expired','payment_exception')), expires_at timestamptz NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
		CREATE INDEX IF NOT EXISTS orders_expiring_idx ON orders(expires_at) WHERE state='pending_payment';
		CREATE TABLE IF NOT EXISTS payment_events(id text PRIMARY KEY,order_id text NOT NULL REFERENCES orders(id),event_type text NOT NULL CHECK(event_type='paid'),event_key text NOT NULL UNIQUE,received_at timestamptz NOT NULL DEFAULT now(),processed_at timestamptz,lease_until timestamptz,attempts integer NOT NULL DEFAULT 0,last_error text);
		CREATE INDEX IF NOT EXISTS payment_events_pending_idx ON payment_events(received_at) WHERE processed_at IS NULL;`
	const simulatedChargesSchema = `CREATE TABLE IF NOT EXISTS simulated_charges(order_id text PRIMARY KEY REFERENCES orders(id),charge_id text NOT NULL UNIQUE,amount_cents bigint NOT NULL CHECK(amount_cents>0),status text NOT NULL CHECK(status IN ('pending','paid')),created_at timestamptz NOT NULL DEFAULT now(),updated_at timestamptz NOT NULL DEFAULT now());`
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(421337)`); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("lock schema migration: %w", err)
	}
	if _, err := tx.Exec(ctx, schema); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migrate schema: %w", err)
	}
	if _, err := tx.Exec(ctx, simulatedChargesSchema); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migrate simulated charges: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds)
		VALUES ('demo-offer','Sacola surpresa — demonstração local',2500,1,1,120) ON CONFLICT(id) DO NOTHING`)
	if err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("seed demo offer: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (s *Store) ListOffers(ctx context.Context) ([]Offer, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,title,price_cents,available_units,reservation_ttl_seconds FROM offers ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Offer
	for rows.Next() {
		var o Offer
		if err := rows.Scan(&o.ID, &o.Title, &o.PriceCents, &o.AvailableUnits, &o.ReservationTTLSecond); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func (s *Store) CreateOrder(ctx context.Context, id, offerID string) (Order, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var price int64
	var ttl int
	err = tx.QueryRow(ctx, `UPDATE offers SET available_units=available_units-1 WHERE id=$1 AND available_units > 0 RETURNING price_cents,reservation_ttl_seconds`, offerID).Scan(&price, &ttl)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrUnavailable
	}
	if err != nil {
		return Order{}, err
	}
	var order Order
	err = tx.QueryRow(ctx, `INSERT INTO orders(id,offer_id,amount_cents,state,expires_at) VALUES($1,$2,$3,'pending_payment',now()+($4 * interval '1 second')) RETURNING id,offer_id,amount_cents,state,expires_at`, id, offerID, price, ttl).Scan(&order.ID, &order.OfferID, &order.Amount, &order.State, &order.ExpiresAt)
	if err != nil {
		return Order{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return order, nil
}

func (s *Store) ExpireOrders(ctx context.Context) (int64, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,offer_id FROM orders WHERE state='pending_payment' AND expires_at <= now() ORDER BY id FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return 0, err
	}
	type expiredOrder struct{ id, offerID string }
	orderIDs := make([]expiredOrder, 0)
	for rows.Next() {
		var id, offerID string
		if err := rows.Scan(&id, &offerID); err != nil {
			rows.Close()
			return 0, err
		}
		orderIDs = append(orderIDs, expiredOrder{id: id, offerID: offerID})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, order := range orderIDs {
		var currentState string
		if err := tx.QueryRow(ctx, `SELECT state FROM orders WHERE id=$1`, order.id).Scan(&currentState); err != nil {
			return 0, err
		}
		if currentState != "pending_payment" {
			continue
		}
		if _, err := tx.Exec(ctx, `UPDATE orders SET state='expired' WHERE id=$1 AND state='pending_payment'`, order.id); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE offers SET available_units=LEAST(total_units,available_units+1) WHERE id=$1`, order.offerID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(orderIDs)), nil
}

func (s *Store) MarkPaid(ctx context.Context, id string) (Order, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var orderID, offerID, state string
	var amount int64
	var expiresAt time.Time
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT id,offer_id,amount_cents,state,expires_at,now() FROM orders WHERE id=$1 FOR UPDATE`, id).Scan(&orderID, &offerID, &amount, &state, &expiresAt, &now)
	if err != nil {
		return Order{}, err
	}
	if state == "paid" || state == "payment_exception" {
		return Order{}, pgx.ErrNoRows
	}
	newState := "paid"
	if state == "expired" || !expiresAt.After(now) {
		newState = "payment_exception"
	}
	var o Order
	err = tx.QueryRow(ctx, `UPDATE orders SET state=$2 WHERE id=$1 RETURNING id,offer_id,amount_cents,state,expires_at`, id, newState).Scan(&o.ID, &o.OfferID, &o.Amount, &o.State, &o.ExpiresAt)
	if err != nil {
		return Order{}, err
	}
	if state == "pending_payment" && newState == "payment_exception" {
		if _, err := tx.Exec(ctx, `UPDATE offers SET available_units=LEAST(total_units,available_units+1) WHERE id=$1`, offerID); err != nil {
			return Order{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, err
	}
	return o, nil
}

func (s *Store) GetOrder(ctx context.Context, id string) (Order, error) {
	var o Order
	err := s.pool.QueryRow(ctx, `SELECT id,offer_id,amount_cents,state,expires_at FROM orders WHERE id=$1`, id).Scan(&o.ID, &o.OfferID, &o.Amount, &o.State, &o.ExpiresAt)
	return o, err
}

func (s *Store) ListOrders(ctx context.Context) ([]Order, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,offer_id,amount_cents,state,expires_at FROM orders ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Order
	for rows.Next() {
		var o Order
		if err := rows.Scan(&o.ID, &o.OfferID, &o.Amount, &o.State, &o.ExpiresAt); err != nil {
			return nil, err
		}
		result = append(result, o)
	}
	return result, rows.Err()
}

func (s *Store) ResetDemo(ctx context.Context) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM payment_events`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM simulated_charges`); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM orders`); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) PersistPaidEvent(ctx context.Context, id, orderID, eventKey string) (bool, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `INSERT INTO payment_events(id,order_id,event_type,event_key) VALUES($1,$2,'paid',$3) ON CONFLICT(event_key) DO NOTHING`, id, orderID, eventKey)
	if err != nil {
		return false, err
	}
	created := tag.RowsAffected() == 1
	var existingOrderID string
	if err := tx.QueryRow(ctx, `SELECT order_id FROM payment_events WHERE event_key=$1`, eventKey).Scan(&existingOrderID); err != nil {
		return false, err
	}
	if existingOrderID != orderID {
		return false, ErrEventKeyConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return created, nil
}

func (s *Store) CreateSimulatedCharge(ctx context.Context, orderID string, amount int64) (SimulatedCharge, error) {
	chargeID := "sim-" + orderID
	if _, err := s.pool.Exec(ctx, `INSERT INTO simulated_charges(order_id,charge_id,amount_cents,status) VALUES($1,$2,$3,'pending') ON CONFLICT(order_id) DO NOTHING`, orderID, chargeID, amount); err != nil {
		return SimulatedCharge{}, err
	}
	charge, err := s.GetSimulatedCharge(ctx, orderID)
	if err != nil {
		return SimulatedCharge{}, err
	}
	if charge.AmountCents != amount {
		return SimulatedCharge{}, ErrChargeConflict
	}
	return charge, nil
}

func (s *Store) GetSimulatedCharge(ctx context.Context, orderID string) (SimulatedCharge, error) {
	var c SimulatedCharge
	err := s.pool.QueryRow(ctx, `SELECT charge_id,order_id,amount_cents,status FROM simulated_charges WHERE order_id=$1`, orderID).Scan(&c.ID, &c.OrderID, &c.AmountCents, &c.Status)
	return c, err
}

func (s *Store) MarkSimulatedChargePaid(ctx context.Context, orderID string) (SimulatedCharge, error) {
	var c SimulatedCharge
	err := s.pool.QueryRow(ctx, `UPDATE simulated_charges SET status='paid',updated_at=now() WHERE order_id=$1 RETURNING charge_id,order_id,amount_cents,status`, orderID).Scan(&c.ID, &c.OrderID, &c.AmountCents, &c.Status)
	return c, err
}

func (s *Store) ProcessPaymentEvents(ctx context.Context, limit int) (int, error) {
	return s.processPaymentEvents(ctx, limit, s.processPaymentEvent)
}

func (s *Store) processPaymentEvents(ctx context.Context, limit int, process func(context.Context, string, string) error) (int, error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,order_id,attempts+1 FROM payment_events WHERE processed_at IS NULL AND (lease_until IS NULL OR lease_until<now()) ORDER BY received_at FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	type event struct {
		id, orderID string
		attempts    int
	}
	var events []event
	for rows.Next() {
		var e event
		if err := rows.Scan(&e.id, &e.orderID, &e.attempts); err != nil {
			rows.Close()
			return 0, err
		}
		events = append(events, e)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	for _, e := range events {
		if _, err := tx.Exec(ctx, `UPDATE payment_events SET lease_until=now()+interval '30 seconds',attempts=$2 WHERE id=$1`, e.id, e.attempts); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	processed := 0
	var processingErrors []error
	for _, e := range events {
		if err := process(ctx, e.id, e.orderID); err != nil {
			backoff := retryBackoff(e.attempts)
			_, releaseErr := s.pool.Exec(ctx, `UPDATE payment_events SET lease_until=now()+($2 * interval '1 second'),last_error='transient processing error' WHERE id=$1 AND processed_at IS NULL`, e.id, int64(backoff.Seconds()))
			if releaseErr != nil {
				processingErrors = append(processingErrors, fmt.Errorf("process payment event: %w; release lease: %v", err, releaseErr))
				continue
			}
			processingErrors = append(processingErrors, fmt.Errorf("process payment event: %w", err))
			continue
		}
		processed++
	}
	return processed, errors.Join(processingErrors...)
}

func retryBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	seconds := 1 << min(attempt-1, 9)
	if seconds > 300 {
		seconds = 300
	}
	return time.Duration(seconds) * time.Second
}

func (s *Store) processPaymentEvent(ctx context.Context, eventID, orderID string) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var processedAt *time.Time
	if err := tx.QueryRow(ctx, `SELECT processed_at FROM payment_events WHERE id=$1 FOR UPDATE`, eventID).Scan(&processedAt); err != nil {
		return err
	}
	if processedAt != nil {
		return tx.Commit(ctx)
	}
	var state, offerID string
	var expiresAt, now time.Time
	if err := tx.QueryRow(ctx, `SELECT state,offer_id,expires_at,now() FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&state, &offerID, &expiresAt, &now); err != nil {
		return err
	}
	newState := "payment_exception"
	if state == "pending_payment" && expiresAt.After(now) {
		newState = "paid"
	}
	if state == "pending_payment" || state == "expired" {
		if _, err := tx.Exec(ctx, `UPDATE orders SET state=$2 WHERE id=$1`, orderID, newState); err != nil {
			return err
		}
	}
	if state == "pending_payment" && newState == "payment_exception" {
		if _, err := tx.Exec(ctx, `UPDATE offers SET available_units=LEAST(total_units,available_units+1) WHERE id=$1`, offerID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE payment_events SET processed_at=now(),lease_until=NULL,last_error=NULL WHERE id=$1`, eventID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
