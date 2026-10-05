package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

var ErrOrderAccessUnauthorized = errors.New("order credential is invalid or expired")

const orderAccessSchema = `CREATE TABLE IF NOT EXISTS order_access_grants (
	token_hash text PRIMARY KEY CHECK(length(token_hash)=64), order_id text NOT NULL UNIQUE REFERENCES orders(id) ON DELETE CASCADE,
	created_at timestamptz NOT NULL DEFAULT clock_timestamp(), expires_at timestamptz NOT NULL CHECK(expires_at>created_at));`

// OrderAccess is returned once at reservation creation, not by order lookups.
// Possession authorizes one local order; it does not authenticate a person.
type OrderAccess struct {
	Order
	AccessToken    string    `json:"order_access_token"`
	TokenExpiresAt time.Time `json:"order_token_expires_at"`
}

func (s *Store) CreateOrderWithAccess(ctx context.Context, id, offerID string) (OrderAccess, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OrderAccess{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	order, err := reserveOrder(ctx, tx, id, offerID)
	if err != nil {
		return OrderAccess{}, err
	}
	var bytes [32]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return OrderAccess{}, err
	}
	token := hex.EncodeToString(bytes[:])
	var expiresAt time.Time
	if err := tx.QueryRow(ctx, `INSERT INTO order_access_grants(token_hash,order_id,expires_at) VALUES($1,$2,clock_timestamp()+interval '20 minutes') RETURNING expires_at`, credentialHash(token), id).Scan(&expiresAt); err != nil {
		return OrderAccess{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return OrderAccess{}, err
	}
	return OrderAccess{Order: order, AccessToken: token, TokenExpiresAt: expiresAt}, nil
}

func (s *Store) GetCustomerOrder(ctx context.Context, orderID, token string) (Order, error) {
	if len(token) != 64 {
		return Order{}, ErrOrderAccessUnauthorized
	}
	var o Order
	err := s.pool.QueryRow(ctx, `SELECT o.id,o.offer_id,o.amount_cents,o.state,o.expires_at,o.picked_up_at FROM orders o JOIN order_access_grants g ON g.order_id=o.id WHERE o.id=$1 AND g.token_hash=$2 AND g.expires_at>clock_timestamp()`, orderID, credentialHash(token)).Scan(&o.ID, &o.OfferID, &o.Amount, &o.State, &o.ExpiresAt, &o.PickedUpAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrOrderAccessUnauthorized
	}
	return o, err
}

func authorizeOrderAccess(ctx context.Context, tx pgx.Tx, orderID, token string) error {
	var allowed bool
	if len(token) != 64 {
		return ErrOrderAccessUnauthorized
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM order_access_grants WHERE order_id=$1 AND token_hash=$2 AND expires_at>clock_timestamp())`, orderID, credentialHash(token)).Scan(&allowed); err != nil {
		return err
	}
	if !allowed {
		return ErrOrderAccessUnauthorized
	}
	return nil
}
