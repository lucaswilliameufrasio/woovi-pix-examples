package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationStore(t *testing.T, units int) *Store {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := fmt.Sprintf("click_collect_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	config.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	s := &Store{pool: pool}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE products SET total_units=$1,available_units=$1 WHERE id='house-cake'`, units); err != nil {
		t.Fatal(err)
	}
	return s
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func reserve(t *testing.T, s *Store, suffix string) Order {
	t.Helper()
	order, err := s.Reserve(context.Background(), "order-"+suffix, "house-cake", digest("access-"+suffix), digest("pickup-"+suffix))
	if err != nil {
		t.Fatal(err)
	}
	return order
}

func TestConcurrentReservationCannotOversellLastUnit(t *testing.T) {
	s := integrationStore(t, 1)
	const clients = 24
	start := make(chan struct{})
	var wait sync.WaitGroup
	var mu sync.Mutex
	created, unavailable := 0, 0
	errs := make([]error, 0)
	for i := 0; i < clients; i++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, err := s.Reserve(context.Background(), fmt.Sprintf("race-%d", index), "house-cake", digest(fmt.Sprintf("a-%d", index)), digest(fmt.Sprintf("p-%d", index)))
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				created++
				return
			}
			if errors.Is(err, ErrUnavailable) {
				unavailable++
				return
			}
			errs = append(errs, err)
		}(i)
	}
	close(start)
	wait.Wait()
	if len(errs) != 0 || created != 1 || unavailable != clients-1 {
		t.Fatalf("created=%d unavailable=%d errors=%v", created, unavailable, errs)
	}
	var count, available int
	if err := s.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM orders),(SELECT available_units FROM products WHERE id='house-cake')`).Scan(&count, &available); err != nil {
		t.Fatal(err)
	}
	if count != 1 || available != 0 {
		t.Fatalf("orders=%d available=%d", count, available)
	}
}

func TestCancellationIsIdempotentAndLatePaymentDoesNotRestoreFulfillment(t *testing.T) {
	s := integrationStore(t, 1)
	ctx := context.Background()
	order := reserve(t, s, "cancel")
	access := digest("access-cancel")
	for range 2 {
		cancelled, err := s.Cancel(ctx, order.ID, access)
		if err != nil {
			t.Fatal(err)
		}
		if cancelled.PaymentState != "cancelled" {
			t.Fatalf("payment state=%s", cancelled.PaymentState)
		}
	}
	if _, err := s.CreateSimulatedCharge(ctx, order.ID, "charge-cancel", "create-cancel"); !errors.Is(err, ErrOrderConflict) {
		t.Fatalf("charge after cancel err=%v", err)
	}
	var available int
	if err := s.pool.QueryRow(ctx, `SELECT available_units FROM products WHERE id='house-cake'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatalf("available=%d", available)
	}
	if _, err := s.GetOrder(ctx, order.ID, digest("wrong-token")); !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("unauthorized lookup err=%v", err)
	}
}

func TestPaymentEventAfterCancellationRecordsExceptionAndKeepsReleasedStock(t *testing.T) {
	s := integrationStore(t, 1)
	ctx := context.Background()
	order := reserve(t, s, "cancel-late-payment")
	if _, err := s.CreateSimulatedCharge(ctx, order.ID, "charge-cancel-late", "create-cancel-late"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Cancel(ctx, order.ID, digest("access-cancel-late-payment")); err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmSimulatedCharge(ctx, order.ID, "paid-cancel-late"); err != nil {
		t.Fatal(err)
	}
	if count, err := s.ProcessPaymentEvents(ctx, 10); err != nil || count != 1 {
		t.Fatalf("processed=%d err=%v", count, err)
	}
	late, err := s.GetOrder(ctx, order.ID, digest("access-cancel-late-payment"))
	if err != nil {
		t.Fatal(err)
	}
	if late.PaymentState != "payment_exception" || late.FulfillmentState != "awaiting_payment" {
		t.Fatalf("late order=%+v", late)
	}
	var available int
	if err := s.pool.QueryRow(ctx, `SELECT available_units FROM products WHERE id='house-cake'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatalf("late payment changed released stock to %d", available)
	}
}

func TestPaymentEventWorkerAndPickupAreAtomicAndOneTime(t *testing.T) {
	s := integrationStore(t, 1)
	ctx := context.Background()
	order := reserve(t, s, "pickup")
	charge, err := s.CreateSimulatedCharge(ctx, order.ID, "charge-pickup", "created-pickup")
	if err != nil {
		t.Fatal(err)
	}
	if charge.AmountCents != order.AmountCents || charge.Status != "pending" {
		t.Fatalf("charge=%+v", charge)
	}
	if err := s.ConfirmSimulatedCharge(ctx, order.ID, "paid-pickup"); err != nil {
		t.Fatal(err)
	}
	poolConfig := s.pool.Config()
	s.Close()
	restartedPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	s.pool = restartedPool
	count, err := s.ProcessPaymentEvents(ctx, 10)
	if err != nil || count != 1 {
		t.Fatalf("processed=%d err=%v", count, err)
	}
	paid, err := s.GetOrder(ctx, order.ID, digest("access-pickup"))
	if err != nil || paid.PaymentState != "paid" || paid.FulfillmentState != "preparing" {
		t.Fatalf("paid=%+v err=%v", paid, err)
	}
	ready, err := s.SetFulfillment(ctx, order.ID, "ready_for_pickup")
	if err != nil || ready.FulfillmentState != "ready_for_pickup" {
		t.Fatalf("ready=%+v err=%v", ready, err)
	}
	picked, err := s.RedeemPickup(ctx, order.ID, digest("pickup-pickup"))
	if err != nil || picked.FulfillmentState != "picked_up" || picked.PickedUpAt == nil {
		t.Fatalf("picked=%+v err=%v", picked, err)
	}
	if _, err := s.RedeemPickup(ctx, order.ID, digest("pickup-pickup")); !errors.Is(err, ErrPickupAlreadyDone) {
		t.Fatalf("duplicate redemption err=%v", err)
	}
}

func TestExpirationReleasesOnceAndLatePersistedPaymentBecomesException(t *testing.T) {
	s := integrationStore(t, 1)
	ctx := context.Background()
	order := reserve(t, s, "late-expired")
	if _, err := s.CreateSimulatedCharge(ctx, order.ID, "charge-late-expired", "created-late-expired"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	count, err := s.ExpireOrders(ctx, 10)
	if err != nil || count != 1 {
		t.Fatalf("expired=%d err=%v", count, err)
	}
	count, err = s.ExpireOrders(ctx, 10)
	if err != nil || count != 0 {
		t.Fatalf("repeat expiration=%d err=%v", count, err)
	}
	if err := s.ConfirmSimulatedCharge(ctx, order.ID, "paid-after-expiry"); err != nil {
		t.Fatal(err)
	}
	if count, err := s.ProcessPaymentEvents(ctx, 10); err != nil || count != 1 {
		t.Fatalf("processed=%d err=%v", count, err)
	}
	late, err := s.GetOrder(ctx, order.ID, digest("access-late-expired"))
	if err != nil {
		t.Fatal(err)
	}
	if late.PaymentState != "payment_exception" || late.FulfillmentState != "awaiting_payment" {
		t.Fatalf("late order=%+v", late)
	}
	var available int
	if err := s.pool.QueryRow(ctx, `SELECT available_units FROM products WHERE id='house-cake'`).Scan(&available); err != nil {
		t.Fatal(err)
	}
	if available != 1 {
		t.Fatalf("late event changed released inventory to %d", available)
	}
}
