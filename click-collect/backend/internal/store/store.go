package store

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

var (
	ErrUnavailable       = errors.New("product unavailable")
	ErrProductNotFound   = errors.New("product not found")
	ErrOrderNotFound     = errors.New("order not found")
	ErrOrderConflict     = errors.New("order state conflict")
	ErrPickupNotEligible = errors.New("order is not ready for pickup")
	ErrPickupAlreadyDone = errors.New("pickup already completed")
	ErrPickupCodeInvalid = errors.New("pickup code invalid")
)

type Product struct {
	ID                   string `json:"id"`
	Title                string `json:"title"`
	Description          string `json:"description"`
	PriceCents           int64  `json:"price_cents"`
	AvailableUnits       int    `json:"available_units"`
	ReservationTTLSecond int    `json:"reservation_ttl_seconds"`
}

type Order struct {
	ID               string     `json:"id"`
	ProductID        string     `json:"product_id"`
	ProductTitle     string     `json:"product_title"`
	AmountCents      int64      `json:"amount_cents"`
	PaymentState     string     `json:"payment_state"`
	FulfillmentState string     `json:"fulfillment_state"`
	ExpiresAt        time.Time  `json:"expires_at"`
	CreatedAt        time.Time  `json:"created_at"`
	PickedUpAt       *time.Time `json:"picked_up_at,omitempty"`
}

type Charge struct {
	ID           string `json:"id"`
	OrderID      string `json:"order_id"`
	AmountCents  int64  `json:"amount_cents"`
	Status       string `json:"status"`
	PaymentState string `json:"payment_state"`
}

type Store struct {
	pool *pgxpool.Pool
}

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL: %w", err)
	}
	config.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() {
	s.pool.Close()
}

func (s *Store) Migrate(ctx context.Context) error {
	contents, err := migrations.ReadFile("migrations/001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(421338)`); err != nil {
		return fmt.Errorf("lock migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version integer PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	var applied bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=1)`).Scan(&applied); err != nil {
		return fmt.Errorf("check migration ledger: %w", err)
	}
	if applied {
		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration check: %w", err)
		}
		return nil
	}
	if _, err := tx.Exec(ctx, string(contents)); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations(version) VALUES (1)`); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}

func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("ping PostgreSQL: %w", err)
	}
	return nil
}

func (s *Store) ListProducts(ctx context.Context) ([]Product, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,title,description,price_cents,available_units,reservation_ttl_seconds FROM products ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("query products: %w", err)
	}
	defer rows.Close()
	products := make([]Product, 0)
	for rows.Next() {
		var product Product
		if err := rows.Scan(&product.ID, &product.Title, &product.Description, &product.PriceCents, &product.AvailableUnits, &product.ReservationTTLSecond); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read products: %w", err)
	}
	return products, nil
}

func (s *Store) transaction(ctx context.Context) (pgx.Tx, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	return tx, nil
}

func (s *Store) Reserve(ctx context.Context, id, productID, accessHash, pickupHash string) (Order, error) {
	tx, err := s.transaction(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var price int64
	var ttl int
	err = tx.QueryRow(ctx, `UPDATE products SET available_units=available_units-1 WHERE id=$1 AND available_units>0 RETURNING price_cents,reservation_ttl_seconds`, productID).Scan(&price, &ttl)
	if errors.Is(err, pgx.ErrNoRows) {
		var exists bool
		if queryErr := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM products WHERE id=$1)`, productID).Scan(&exists); queryErr != nil {
			return Order{}, fmt.Errorf("check product: %w", queryErr)
		}
		if !exists {
			return Order{}, ErrProductNotFound
		}
		return Order{}, ErrUnavailable
	}
	if err != nil {
		return Order{}, fmt.Errorf("reserve product: %w", err)
	}
	order, err := scanOrder(tx.QueryRow(ctx, `INSERT INTO orders(id,product_id,amount_cents,payment_state,fulfillment_state,expires_at,access_token_hash,access_expires_at,pickup_code_hash)
		VALUES($1,$2,$3,'pending','awaiting_payment',now()+($4 * interval '1 second'),$5,now()+interval '24 hours',$6)
		RETURNING id,product_id,(SELECT title FROM products WHERE id=product_id),amount_cents,payment_state,fulfillment_state,expires_at,created_at,picked_up_at`, id, productID, price, ttl, accessHash, pickupHash))
	if err != nil {
		return Order{}, fmt.Errorf("create order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit reservation: %w", err)
	}
	return order, nil
}

type rowScanner interface{ Scan(...any) error }

func scanOrder(row rowScanner) (Order, error) {
	var order Order
	err := row.Scan(&order.ID, &order.ProductID, &order.ProductTitle, &order.AmountCents, &order.PaymentState, &order.FulfillmentState, &order.ExpiresAt, &order.CreatedAt, &order.PickedUpAt)
	return order, err
}

const orderColumns = `id,product_id,(SELECT title FROM products WHERE id=product_id),amount_cents,payment_state,fulfillment_state,expires_at,created_at,picked_up_at`

func (s *Store) GetOrder(ctx context.Context, id, accessHash string) (Order, error) {
	order, err := scanOrder(s.pool.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1 AND access_token_hash=$2 AND access_expires_at>now()`, id, accessHash))
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("get authorized order: %w", err)
	}
	return order, nil
}

func (s *Store) Cancel(ctx context.Context, id, accessHash string) (Order, error) {
	tx, err := s.transaction(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var productID, paymentState string
	err = tx.QueryRow(ctx, `SELECT product_id,payment_state FROM orders WHERE id=$1 AND access_token_hash=$2 AND access_expires_at>now() FOR UPDATE`, id, accessHash).Scan(&productID, &paymentState)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock order for cancellation: %w", err)
	}
	if paymentState == "cancelled" {
		order, scanErr := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
		if scanErr != nil {
			return Order{}, fmt.Errorf("read cancelled order: %w", scanErr)
		}
		if err := tx.Commit(ctx); err != nil {
			return Order{}, fmt.Errorf("commit idempotent cancellation: %w", err)
		}
		return order, nil
	}
	if paymentState != "pending" {
		return Order{}, ErrOrderConflict
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET payment_state='cancelled',updated_at=now() WHERE id=$1`, id); err != nil {
		return Order{}, fmt.Errorf("cancel order: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE products SET available_units=LEAST(total_units,available_units+1) WHERE id=$1`, productID); err != nil {
		return Order{}, fmt.Errorf("release product: %w", err)
	}
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
	if err != nil {
		return Order{}, fmt.Errorf("read cancelled order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit cancellation: %w", err)
	}
	return order, nil
}

func (s *Store) MarkPaid(ctx context.Context, id string) (Order, error) {
	tx, err := s.transaction(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var paymentState string
	var expiresAt time.Time
	var now time.Time
	err = tx.QueryRow(ctx, `SELECT payment_state,expires_at,now() FROM orders WHERE id=$1 FOR UPDATE`, id).Scan(&paymentState, &expiresAt, &now)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock order for payment: %w", err)
	}
	if paymentState == "paid" || paymentState == "payment_exception" {
		return Order{}, ErrOrderConflict
	}
	newState := "paid"
	if paymentState != "pending" || !expiresAt.After(now) {
		newState = "payment_exception"
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET payment_state=$2,fulfillment_state=CASE WHEN $2='paid' THEN 'preparing' ELSE fulfillment_state END,updated_at=now() WHERE id=$1`, id, newState); err != nil {
		return Order{}, fmt.Errorf("record payment state: %w", err)
	}
	if paymentState == "pending" && newState == "payment_exception" {
		if _, err := tx.Exec(ctx, `UPDATE products SET available_units=LEAST(total_units,available_units+1) WHERE id=(SELECT product_id FROM orders WHERE id=$1)`, id); err != nil {
			return Order{}, fmt.Errorf("release expired reservation: %w", err)
		}
	}
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
	if err != nil {
		return Order{}, fmt.Errorf("read paid order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit payment state: %w", err)
	}
	return order, nil
}

func (s *Store) ExpireOrders(ctx context.Context, limit int) (int64, error) {
	if limit < 1 || limit > 500 {
		return 0, fmt.Errorf("expiration batch limit must be between 1 and 500")
	}
	tx, err := s.transaction(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,product_id FROM orders WHERE payment_state='pending' AND expires_at<=now() ORDER BY expires_at,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("select expired orders: %w", err)
	}
	type expired struct{ id, productID string }
	expiredOrders := make([]expired, 0)
	for rows.Next() {
		var item expired
		if err := rows.Scan(&item.id, &item.productID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan expired order: %w", err)
		}
		expiredOrders = append(expiredOrders, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("read expired orders: %w", err)
	}
	rows.Close()
	for _, item := range expiredOrders {
		if _, err := tx.Exec(ctx, `UPDATE orders SET payment_state='expired',updated_at=now() WHERE id=$1 AND payment_state='pending'`, item.id); err != nil {
			return 0, fmt.Errorf("expire order: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE products SET available_units=LEAST(total_units,available_units+1) WHERE id=$1`, item.productID); err != nil {
			return 0, fmt.Errorf("release expired product: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit expiration: %w", err)
	}
	return int64(len(expiredOrders)), nil
}

func (s *Store) OperatorQueue(ctx context.Context) ([]Order, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+orderColumns+` FROM orders WHERE payment_state='paid' ORDER BY created_at DESC,id LIMIT 100`)
	if err != nil {
		return nil, fmt.Errorf("query operator queue: %w", err)
	}
	defer rows.Close()
	orders := make([]Order, 0)
	for rows.Next() {
		order, err := scanOrder(rows)
		if err != nil {
			return nil, fmt.Errorf("scan operator order: %w", err)
		}
		orders = append(orders, order)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read operator queue: %w", err)
	}
	return orders, nil
}

func (s *Store) SetFulfillment(ctx context.Context, id, target string) (Order, error) {
	if target != "preparing" && target != "ready_for_pickup" {
		return Order{}, ErrOrderConflict
	}
	tx, err := s.transaction(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state, payment string
	err = tx.QueryRow(ctx, `SELECT fulfillment_state,payment_state FROM orders WHERE id=$1 FOR UPDATE`, id).Scan(&state, &payment)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock fulfillment order: %w", err)
	}
	if payment != "paid" || state != "preparing" {
		if payment == "paid" && state == target {
			order, scanErr := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
			if scanErr != nil {
				return Order{}, fmt.Errorf("read unchanged fulfillment: %w", scanErr)
			}
			if err := tx.Commit(ctx); err != nil {
				return Order{}, fmt.Errorf("commit unchanged fulfillment: %w", err)
			}
			return order, nil
		}
		return Order{}, ErrOrderConflict
	}
	if target == "preparing" {
		return scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET fulfillment_state=$2,updated_at=now() WHERE id=$1`, id, target); err != nil {
		return Order{}, fmt.Errorf("update fulfillment: %w", err)
	}
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
	if err != nil {
		return Order{}, fmt.Errorf("read fulfillment order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit fulfillment: %w", err)
	}
	return order, nil
}

func (s *Store) RedeemPickup(ctx context.Context, id, codeHash string) (Order, error) {
	tx, err := s.transaction(ctx)
	if err != nil {
		return Order{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state, payment, storedHash string
	err = tx.QueryRow(ctx, `SELECT fulfillment_state,payment_state,pickup_code_hash FROM orders WHERE id=$1 FOR UPDATE`, id).Scan(&state, &payment, &storedHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("lock pickup order: %w", err)
	}
	if storedHash != codeHash {
		return Order{}, ErrPickupCodeInvalid
	}
	if state == "picked_up" {
		return Order{}, ErrPickupAlreadyDone
	}
	if payment != "paid" || state != "ready_for_pickup" {
		return Order{}, ErrPickupNotEligible
	}
	if _, err := tx.Exec(ctx, `UPDATE orders SET fulfillment_state='picked_up',pickup_consumed_at=now(),picked_up_at=now(),updated_at=now() WHERE id=$1`, id); err != nil {
		return Order{}, fmt.Errorf("consume pickup code: %w", err)
	}
	order, err := scanOrder(tx.QueryRow(ctx, `SELECT `+orderColumns+` FROM orders WHERE id=$1`, id))
	if err != nil {
		return Order{}, fmt.Errorf("read picked-up order: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Order{}, fmt.Errorf("commit pickup: %w", err)
	}
	return order, nil
}

func (s *Store) CreateSimulatedCharge(ctx context.Context, orderID, chargeID, eventKey string) (Charge, error) {
	tx, err := s.transaction(ctx)
	if err != nil {
		return Charge{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var amount int64
	var payment string
	err = tx.QueryRow(ctx, `SELECT amount_cents,payment_state FROM orders WHERE id=$1 FOR UPDATE`, orderID).Scan(&amount, &payment)
	if errors.Is(err, pgx.ErrNoRows) {
		return Charge{}, ErrOrderNotFound
	}
	if err != nil {
		return Charge{}, fmt.Errorf("lock order for simulated charge: %w", err)
	}
	if payment != "pending" {
		return Charge{}, ErrOrderConflict
	}
	var charge Charge
	err = tx.QueryRow(ctx, `INSERT INTO simulated_charges(order_id,charge_id,amount_cents,status,event_key) VALUES($1,$2,$3,'pending',$4)
		ON CONFLICT(order_id) DO UPDATE SET updated_at=now()
		RETURNING charge_id,order_id,amount_cents,status`, orderID, chargeID, amount, eventKey).Scan(&charge.ID, &charge.OrderID, &charge.AmountCents, &charge.Status)
	if err != nil {
		return Charge{}, fmt.Errorf("create simulated charge: %w", err)
	}
	charge.PaymentState = payment
	if err := tx.Commit(ctx); err != nil {
		return Charge{}, fmt.Errorf("commit simulated charge: %w", err)
	}
	return charge, nil
}

func (s *Store) ConfirmSimulatedCharge(ctx context.Context, orderID, eventKey string) error {
	tx, err := s.transaction(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var chargeID string
	err = tx.QueryRow(ctx, `SELECT charge_id FROM simulated_charges WHERE order_id=$1 FOR UPDATE`, orderID).Scan(&chargeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("lock simulated charge: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_events(order_id,event_key) VALUES($1,$2) ON CONFLICT(event_key) DO NOTHING`, orderID, eventKey); err != nil {
		return fmt.Errorf("persist simulated payment event: %w", err)
	}
	if _, err := tx.Exec(ctx, `UPDATE simulated_charges SET status='paid',updated_at=now() WHERE order_id=$1`, orderID); err != nil {
		return fmt.Errorf("confirm simulated charge: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit simulated payment event: %w", err)
	}
	return nil
}

func (s *Store) SimulateAuthorizedPayment(ctx context.Context, orderID, accessHash string) error {
	tx, err := s.transaction(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var amount int64
	var paymentState string
	err = tx.QueryRow(ctx, `SELECT amount_cents,payment_state FROM orders WHERE id=$1 AND access_token_hash=$2 AND access_expires_at>now() FOR UPDATE`, orderID, accessHash).Scan(&amount, &paymentState)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOrderNotFound
	}
	if err != nil {
		return fmt.Errorf("lock order for local payment simulation: %w", err)
	}
	if paymentState == "paid" {
		return tx.Commit(ctx)
	}
	if paymentState != "pending" {
		return ErrOrderConflict
	}
	chargeID := "demo_" + orderID
	eventKey := "demo_paid_" + orderID
	if _, err := tx.Exec(ctx, `INSERT INTO simulated_charges(order_id,charge_id,amount_cents,status,event_key) VALUES($1,$2,$3,'paid',$4)
		ON CONFLICT(order_id) DO UPDATE SET status='paid',updated_at=now()`, orderID, chargeID, amount, "created_"+orderID); err != nil {
		return fmt.Errorf("record local simulated charge: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO payment_events(order_id,event_key) VALUES($1,$2) ON CONFLICT(event_key) DO NOTHING`, orderID, eventKey); err != nil {
		return fmt.Errorf("persist local simulated event: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit local simulated payment: %w", err)
	}
	return nil
}

func (s *Store) ProcessPaymentEvents(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 500 {
		return 0, fmt.Errorf("payment event batch limit must be between 1 and 500")
	}
	tx, err := s.transaction(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT id,order_id FROM payment_events WHERE processed_at IS NULL ORDER BY received_at,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("select payment events: %w", err)
	}
	type paymentEvent struct {
		id      int64
		orderID string
	}
	events := make([]paymentEvent, 0)
	for rows.Next() {
		var event paymentEvent
		if err := rows.Scan(&event.id, &event.orderID); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan payment event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("read payment events: %w", err)
	}
	rows.Close()
	for _, event := range events {
		var payment string
		var expires, now time.Time
		if err := tx.QueryRow(ctx, `SELECT payment_state,expires_at,now() FROM orders WHERE id=$1 FOR UPDATE`, event.orderID).Scan(&payment, &expires, &now); err != nil {
			return 0, fmt.Errorf("lock payment order: %w", err)
		}
		if payment == "pending" || payment == "cancelled" || payment == "expired" {
			next := "paid"
			if payment != "pending" || !expires.After(now) {
				next = "payment_exception"
			}
			if _, err := tx.Exec(ctx, `UPDATE orders SET payment_state=$2,fulfillment_state=CASE WHEN $2='paid' THEN 'preparing' ELSE fulfillment_state END,updated_at=now() WHERE id=$1`, event.orderID, next); err != nil {
				return 0, fmt.Errorf("apply payment event: %w", err)
			}
			if payment == "pending" && next == "payment_exception" {
				if _, err := tx.Exec(ctx, `UPDATE products SET available_units=LEAST(total_units,available_units+1) WHERE id=(SELECT product_id FROM orders WHERE id=$1)`, event.orderID); err != nil {
					return 0, fmt.Errorf("release late-payment reservation: %w", err)
				}
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE payment_events SET processed_at=now(),attempts=attempts+1,last_error=NULL WHERE id=$1`, event.id); err != nil {
			return 0, fmt.Errorf("acknowledge payment event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit payment events: %w", err)
	}
	return len(events), nil
}
