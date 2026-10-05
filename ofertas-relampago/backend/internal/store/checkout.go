package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrCheckoutKeyConflict = errors.New("checkout key belongs to another order")
var ErrCheckoutNotEligible = errors.New("order cannot start a local checkout")
var ErrCheckoutUnauthorized = errors.New("checkout credential is invalid or expired")
var ErrCheckoutPaymentException = errors.New("payment needs manual review")

const localCheckoutSchema = `CREATE TABLE IF NOT EXISTS local_checkout_sessions (
	checkout_id text PRIMARY KEY, order_id text NOT NULL UNIQUE REFERENCES simulated_charges(order_id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT now());
	CREATE TABLE IF NOT EXISTS local_checkout_keys (
	key_hash text PRIMARY KEY CHECK(length(key_hash)=64), checkout_id text NOT NULL REFERENCES local_checkout_sessions(checkout_id) ON DELETE CASCADE);
	CREATE TABLE IF NOT EXISTS local_checkout_grants (
	token_hash text PRIMARY KEY CHECK(length(token_hash)=64), checkout_id text NOT NULL REFERENCES local_checkout_sessions(checkout_id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL CHECK(expires_at>created_at));
	CREATE INDEX IF NOT EXISTS local_checkout_grants_session_idx ON local_checkout_grants(checkout_id);`

// LocalCheckout deliberately contains no payable Pix payload. Payment state and
// fulfillment remain authoritative on the order; the session grants only read access.
type LocalCheckout struct {
	ID           string    `json:"checkout_id"`
	OrderID      string    `json:"order_id"`
	ChargeID     string    `json:"charge_id"`
	AmountCents  int64     `json:"amount_cents"`
	Status       string    `json:"status"`
	ChargeStatus string    `json:"charge_status"`
	OrderState   string    `json:"order_state"`
	ExpiresAt    time.Time `json:"expires_at"`
}

type LocalCheckoutGrant struct {
	LocalCheckout
	AccessToken    string    `json:"access_token"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

func credentialHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

// IssueLocalCheckout is called only after merchant/operator authorization.
// Local charge, session and key are committed atomically. There is no PSP call
// inside or outside this transaction, and a replay cannot reserve more stock.
func (s *Store) IssueLocalCheckout(ctx context.Context, orderID, key string) (LocalCheckoutGrant, bool, error) {
	return s.issueLocalCheckout(ctx, orderID, key, "")
}

func (s *Store) IssueCustomerLocalCheckout(ctx context.Context, orderID, key, token string) (LocalCheckoutGrant, bool, error) {
	if len(token) != 64 {
		return LocalCheckoutGrant{}, false, ErrOrderAccessUnauthorized
	}
	return s.issueLocalCheckout(ctx, orderID, key, token)
}

func (s *Store) issueLocalCheckout(ctx context.Context, orderID, key, orderToken string) (LocalCheckoutGrant, bool, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if orderToken != "" {
		if err := authorizeOrderAccess(ctx, tx, orderID, orderToken); err != nil {
			return LocalCheckoutGrant{}, false, err
		}
	}
	keyHash := credentialHash(key)
	// Key first, then order: cross-order key reuse and cross-process callers
	// serialize without introducing an order->key lock inversion.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "local-checkout-key:"+keyHash); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	var boundOrder string
	err = tx.QueryRow(ctx, `SELECT c.order_id FROM local_checkout_keys k JOIN local_checkout_sessions c USING(checkout_id) WHERE k.key_hash=$1`, keyHash).Scan(&boundOrder)
	if err == nil && boundOrder != orderID {
		return LocalCheckoutGrant{}, false, ErrCheckoutKeyConflict
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return LocalCheckoutGrant{}, false, err
	}
	var order Order
	if err := tx.QueryRow(ctx, `SELECT id,amount_cents,state,expires_at FROM orders WHERE id=$1 FOR UPDATE`, orderID).
		Scan(&order.ID, &order.Amount, &order.State, &order.ExpiresAt); err != nil {
		if orderToken != "" && errors.Is(err, pgx.ErrNoRows) {
			return LocalCheckoutGrant{}, false, ErrOrderAccessUnauthorized
		}
		return LocalCheckoutGrant{}, false, err
	}
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT clock_timestamp()`).Scan(&now); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	// Recheck after potentially waiting on key/order locks. An expired grant
	// must not create/renew a checkout even if valid when the request arrived.
	if orderToken != "" {
		if err := authorizeOrderAccess(ctx, tx, orderID, orderToken); err != nil {
			return LocalCheckoutGrant{}, false, err
		}
	}
	if order.State == "payment_exception" {
		return LocalCheckoutGrant{}, false, ErrCheckoutPaymentException
	}
	var checkoutID string
	err = tx.QueryRow(ctx, `SELECT checkout_id FROM local_checkout_sessions WHERE order_id=$1`, orderID).Scan(&checkoutID)
	created := errors.Is(err, pgx.ErrNoRows)
	if err != nil && !created {
		return LocalCheckoutGrant{}, false, err
	}
	if created {
		if order.State != "pending_payment" || !order.ExpiresAt.After(now) {
			return LocalCheckoutGrant{}, false, ErrCheckoutNotEligible
		}
		if _, err := tx.Exec(ctx, `INSERT INTO simulated_charges(order_id,charge_id,amount_cents,status) VALUES($1,$2,$3,'pending') ON CONFLICT(order_id) DO NOTHING`, orderID, "sim-"+orderID, order.Amount); err != nil {
			return LocalCheckoutGrant{}, false, err
		}
		var chargeAmount int64
		var chargeStatus string
		if err := tx.QueryRow(ctx, `SELECT amount_cents,status FROM simulated_charges WHERE order_id=$1 FOR UPDATE`, orderID).Scan(&chargeAmount, &chargeStatus); err != nil {
			return LocalCheckoutGrant{}, false, err
		}
		if chargeAmount != order.Amount {
			return LocalCheckoutGrant{}, false, ErrChargeConflict
		}
		// A paid charge still awaiting its worker must not open a new checkout.
		if chargeStatus != "pending" {
			return LocalCheckoutGrant{}, false, ErrCheckoutNotEligible
		}
		checkoutID, err = randomCheckoutID()
		if err != nil {
			return LocalCheckoutGrant{}, false, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO local_checkout_sessions(checkout_id,order_id) VALUES($1,$2)`, checkoutID, orderID); err != nil {
			return LocalCheckoutGrant{}, false, err
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO local_checkout_keys(key_hash,checkout_id) VALUES($1,$2) ON CONFLICT(key_hash) DO NOTHING`, keyHash, checkoutID); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	var tokenBytes [32]byte
	if _, err := rand.Read(tokenBytes[:]); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	token := hex.EncodeToString(tokenBytes[:])
	var tokenExpiresAt time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO local_checkout_grants(token_hash,checkout_id,expires_at) VALUES($1,$2,clock_timestamp()+interval '10 minutes') RETURNING expires_at`, credentialHash(token), checkoutID).Scan(&tokenExpiresAt); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	checkout, err := readLocalCheckout(ctx, tx, checkoutID)
	if err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LocalCheckoutGrant{}, false, err
	}
	return LocalCheckoutGrant{LocalCheckout: checkout, AccessToken: token, TokenExpiresAt: tokenExpiresAt}, created, nil
}

func randomCheckoutID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	// UUID v4. No secret is encoded in this identifier.
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	h := hex.EncodeToString(id[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:], nil
}

// GetLocalCheckout intentionally returns the same error for absent sessions,
// expired grants and wrong-session credentials. Tokens cannot authorize writes.
func (s *Store) GetLocalCheckout(ctx context.Context, checkoutID, token string) (LocalCheckout, error) {
	if len(token) != 64 {
		return LocalCheckout{}, ErrCheckoutUnauthorized
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return LocalCheckout{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var allowed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM local_checkout_grants WHERE checkout_id=$1 AND token_hash=$2 AND expires_at>clock_timestamp())`, checkoutID, credentialHash(token)).Scan(&allowed); err != nil {
		return LocalCheckout{}, err
	}
	if !allowed {
		return LocalCheckout{}, ErrCheckoutUnauthorized
	}
	checkout, err := readLocalCheckout(ctx, tx, checkoutID)
	if errors.Is(err, pgx.ErrNoRows) {
		return LocalCheckout{}, ErrCheckoutUnauthorized
	}
	if err != nil {
		return LocalCheckout{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return LocalCheckout{}, err
	}
	return checkout, nil
}

func readLocalCheckout(ctx context.Context, tx pgx.Tx, id string) (LocalCheckout, error) {
	var c LocalCheckout
	var chargeAmount int64
	var now time.Time
	err := tx.QueryRow(ctx, `SELECT c.checkout_id,c.order_id,p.charge_id,o.amount_cents,p.amount_cents,o.state,p.status,o.expires_at,clock_timestamp()
		FROM local_checkout_sessions c JOIN orders o ON o.id=c.order_id JOIN simulated_charges p ON p.order_id=c.order_id WHERE c.checkout_id=$1`, id).
		Scan(&c.ID, &c.OrderID, &c.ChargeID, &c.AmountCents, &chargeAmount, &c.OrderState, &c.ChargeStatus, &c.ExpiresAt, &now)
	if err != nil {
		return LocalCheckout{}, err
	}
	if chargeAmount != c.AmountCents {
		return LocalCheckout{}, ErrChargeConflict
	}
	switch c.OrderState {
	case "payment_exception":
		return LocalCheckout{}, ErrCheckoutPaymentException
	case "paid":
		c.Status = "paid"
	case "expired":
		c.Status = "expired"
	default:
		c.Status = "pending"
		if !c.ExpiresAt.After(now) {
			c.Status = "expired"
		}
	}
	return c, nil
}
