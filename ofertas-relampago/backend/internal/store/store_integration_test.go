package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"
)

func integrationStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	offerID := testID(t)
	t.Cleanup(func() {
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM orders WHERE offer_id=$1`, offerID)
		_, _ = s.pool.Exec(context.Background(), `UPDATE offers SET available_units=total_units WHERE id=$1`, offerID)
		_, _ = s.pool.Exec(context.Background(), `DELETE FROM offers WHERE id=$1`, offerID)
	})
	if _, err := s.pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Integration offer',2500,1,1,120)`, offerID); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_OFFER_ID", offerID)
	return s
}

func testID(t *testing.T) string {
	t.Helper()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	return "test-" + hex.EncodeToString(b[:])
}

func offerID(t *testing.T) string { t.Helper(); return os.Getenv("TEST_OFFER_ID") }

func TestConcurrentLastUnitOnlyCreatesOneOrder(t *testing.T) {
	s := integrationStore(t)
	const clients = 24
	start := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	success, unavailable := 0, 0
	errs := make([]error, 0)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, err := s.CreateOrder(context.Background(), offerID(t)+"-race-"+testID(t), offerID(t))
			mu.Lock()
			defer mu.Unlock()
			switch err {
			case nil:
				success++
			case ErrUnavailable:
				unavailable++
			default:
				errs = append(errs, err)
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if success != 1 || unavailable != clients-1 {
		t.Fatalf("success=%d unavailable=%d", success, unavailable)
	}
	var count, units int
	if err := s.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM orders WHERE offer_id=$1),(SELECT available_units FROM offers WHERE id=$1)`, offerID(t)).Scan(&count, &units); err != nil {
		t.Fatal(err)
	}
	if count != 1 || units != 0 {
		t.Fatalf("orders=%d available=%d", count, units)
	}
}

func TestExpiryReleasesOnceAndLatePaymentIsException(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-expire", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	count, err := s.ExpireOrders(context.Background())
	if err != nil || count < 1 {
		t.Fatalf("expired count=%d err=%v", count, err)
	}
	count, err = s.ExpireOrders(context.Background())
	if err != nil {
		t.Fatalf("second expiration count=%d err=%v", count, err)
	}
	var units int
	if err := s.pool.QueryRow(context.Background(), `SELECT available_units FROM offers WHERE id=$1`, offerID(t)).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if units != 1 {
		t.Fatalf("available=%d", units)
	}
	paid, err := s.MarkPaid(context.Background(), order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.State != "payment_exception" {
		t.Fatalf("late payment state=%q", paid.State)
	}
}

func TestLatePaymentWhenPendingReleasesReservedStockAtomically(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-late-pending", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	paid, err := s.MarkPaid(context.Background(), order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.State != "payment_exception" {
		t.Fatalf("late state=%s", paid.State)
	}
	var units int
	if err := s.pool.QueryRow(context.Background(), `SELECT available_units FROM offers WHERE id=$1`, offerID(t)).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if units != 1 {
		t.Fatalf("late payment left available units=%d", units)
	}
}

func TestLatePaymentDoesNotIncrementStockAfterAnotherOrderTakesReleasedUnit(t *testing.T) {
	s := integrationStore(t)
	first, err := s.CreateOrder(context.Background(), offerID(t)+"-first", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ExpireOrders(context.Background()); err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateOrder(context.Background(), offerID(t)+"-second", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	late, err := s.MarkPaid(context.Background(), first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if late.State != "payment_exception" {
		t.Fatalf("late order state=%s", late.State)
	}
	var units int
	if err := s.pool.QueryRow(context.Background(), `SELECT available_units FROM offers WHERE id=$1`, offerID(t)).Scan(&units); err != nil {
		t.Fatal(err)
	}
	if units != 0 {
		t.Fatalf("late payment made stock available while second order %s holds it: available=%d", second.ID, units)
	}
}

func TestExpirationAndPaidRaceDoesNotLoseUnit(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-payment", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	var expireErr, payErr error
	go func() { defer wg.Done(); <-start; _, expireErr = s.ExpireOrders(context.Background()) }()
	go func() { defer wg.Done(); <-start; _, payErr = s.MarkPaid(context.Background(), order.ID) }()
	close(start)
	wg.Wait()
	if expireErr != nil {
		t.Fatal(expireErr)
	}
	if payErr != nil {
		t.Fatal(payErr)
	}
	var state string
	var units int
	if err := s.pool.QueryRow(context.Background(), `SELECT (SELECT state FROM orders WHERE id=$1),(SELECT available_units FROM offers WHERE id=$2)`, order.ID, offerID(t)).Scan(&state, &units); err != nil {
		t.Fatal(err)
	}
	if state != "payment_exception" || units != 1 {
		t.Fatalf("state=%s units=%d", state, units)
	}
}

func TestWebhookDedupAndWorkerProcessOnce(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-webhook", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	eventKey := "event-unique-" + testID(t)
	created, err := s.PersistPaidEvent(context.Background(), testID(t), order.ID, eventKey)
	if err != nil || !created {
		t.Fatalf("first event created=%v err=%v", created, err)
	}
	duplicate, err := s.PersistPaidEvent(context.Background(), testID(t), order.ID, eventKey)
	if err != nil {
		t.Fatal(err)
	}
	if duplicate {
		t.Fatal("duplicate webhook event was inserted twice")
	}
	processed, err := s.ProcessPaymentEvents(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d err=%v", processed, err)
	}
	processed, err = s.ProcessPaymentEvents(context.Background(), 10)
	if err != nil || processed != 0 {
		t.Fatalf("second worker processed=%d err=%v", processed, err)
	}
	paid, err := s.GetOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.State != "paid" {
		t.Fatalf("order state=%s", paid.State)
	}
}

func TestWebhookEventKeyCannotBeReusedAcrossOrders(t *testing.T) {
	s := integrationStore(t)
	if _, err := s.pool.Exec(context.Background(), `UPDATE offers SET total_units=2,available_units=2 WHERE id=$1`, offerID(t)); err != nil {
		t.Fatal(err)
	}
	first, err := s.CreateOrder(context.Background(), offerID(t)+"-event-first", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.CreateOrder(context.Background(), offerID(t)+"-event-second", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	eventKey := "unique-event-" + testID(t)
	if inserted, err := s.PersistPaidEvent(context.Background(), testID(t), first.ID, eventKey); err != nil || !inserted {
		t.Fatalf("first insert=%v err=%v", inserted, err)
	}
	if _, err := s.PersistPaidEvent(context.Background(), testID(t), second.ID, eventKey); !errors.Is(err, ErrEventKeyConflict) {
		t.Fatalf("expected event-key conflict, got %v", err)
	}
}

func TestWorkerRecoversEventAfterExpiredLease(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-lease", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	eventID, eventKey := testID(t), "lease-event-"+testID(t)
	if created, err := s.PersistPaidEvent(context.Background(), eventID, order.ID, eventKey); err != nil || !created {
		t.Fatalf("persist event created=%v err=%v", created, err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE payment_events SET attempts=1,lease_until=now()+interval '30 seconds' WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	processed, err := s.ProcessPaymentEvents(context.Background(), 10)
	if err != nil || processed != 0 {
		t.Fatalf("active lease processed=%d err=%v", processed, err)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE payment_events SET lease_until=now()-interval '1 second' WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	processed, err = s.ProcessPaymentEvents(context.Background(), 10)
	if err != nil || processed != 1 {
		t.Fatalf("recovered lease processed=%d err=%v", processed, err)
	}
	var attempts int
	var processedAt *time.Time
	if err := s.pool.QueryRow(context.Background(), `SELECT attempts,processed_at FROM payment_events WHERE id=$1`, eventID).Scan(&attempts, &processedAt); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || processedAt == nil {
		t.Fatalf("attempts=%d processed_at=%v", attempts, processedAt)
	}
}

func TestRetryBackoffIsBounded(t *testing.T) {
	for attempt, want := range map[int]time.Duration{1: time.Second, 2: 2 * time.Second, 9: 256 * time.Second, 10: 300 * time.Second, 100: 300 * time.Second} {
		if got := retryBackoff(attempt); got != want {
			t.Errorf("retryBackoff(%d)=%s want %s", attempt, got, want)
		}
	}
}

func TestTransientWorkerFailureSchedulesRetryAndThenProcesses(t *testing.T) {
	s := integrationStore(t)
	order, err := s.CreateOrder(context.Background(), offerID(t)+"-retry", offerID(t))
	if err != nil {
		t.Fatal(err)
	}
	eventID, eventKey := testID(t), "retry-event-"+testID(t)
	if inserted, err := s.PersistPaidEvent(context.Background(), eventID, order.ID, eventKey); err != nil || !inserted {
		t.Fatalf("persist event inserted=%v err=%v", inserted, err)
	}
	processed, err := s.processPaymentEvents(context.Background(), 10, func(context.Context, string, string) error { return fmt.Errorf("temporary injected failure") })
	if processed != 0 || err == nil {
		t.Fatalf("failed pass processed=%d err=%v", processed, err)
	}
	var attempts int
	var lastError string
	var leaseUntil time.Time
	if err := s.pool.QueryRow(context.Background(), `SELECT attempts,last_error,lease_until FROM payment_events WHERE id=$1`, eventID).Scan(&attempts, &lastError, &leaseUntil); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || lastError == "" || !leaseUntil.After(time.Now()) {
		t.Fatalf("attempts=%d last_error=%q lease_until=%s", attempts, lastError, leaseUntil)
	}
	if _, err := s.pool.Exec(context.Background(), `UPDATE payment_events SET lease_until=now()-interval '1 second' WHERE id=$1`, eventID); err != nil {
		t.Fatal(err)
	}
	processed, err = s.ProcessPaymentEvents(context.Background(), 10)
	if processed != 1 || err != nil {
		t.Fatalf("retry pass processed=%d err=%v", processed, err)
	}
	var finished bool
	if err := s.pool.QueryRow(context.Background(), `SELECT processed_at IS NOT NULL AND lease_until IS NULL AND last_error IS NULL AND attempts=2 FROM payment_events WHERE id=$1`, eventID).Scan(&finished); err != nil {
		t.Fatal(err)
	}
	if !finished {
		t.Fatal("retry did not finish and clear lease/error")
	}
	paid, err := s.GetOrder(context.Background(), order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.State != "paid" {
		t.Fatalf("order state after retry=%s", paid.State)
	}
}

func TestConcurrentWorkersClaimEachPaymentEventOnce(t *testing.T) {
	s := integrationStore(t)
	offer := offerID(t)
	if _, err := s.pool.Exec(context.Background(), `UPDATE offers SET total_units=20,available_units=20 WHERE id=$1`, offer); err != nil {
		t.Fatal(err)
	}
	const events = 20
	for i := 0; i < events; i++ {
		id := offer + "-worker-" + testID(t)
		order, err := s.CreateOrder(context.Background(), id, offer)
		if err != nil {
			t.Fatal(err)
		}
		if inserted, err := s.PersistPaidEvent(context.Background(), testID(t), order.ID, "parallel-"+testID(t)); err != nil || !inserted {
			t.Fatalf("persist event inserted=%v err=%v", inserted, err)
		}
	}
	start := make(chan struct{})
	results := make(chan int, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			<-start
			n, err := s.ProcessPaymentEvents(context.Background(), events)
			results <- n
			errs <- err
		}()
	}
	close(start)
	processed := 0
	for i := 0; i < 2; i++ {
		processed += <-results
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	if processed != events {
		t.Fatalf("claimed/processed events=%d want %d", processed, events)
	}
	var completed, wrongAttempts int
	if err := s.pool.QueryRow(context.Background(), `SELECT count(*) FILTER(WHERE processed_at IS NOT NULL),count(*) FILTER(WHERE attempts<>1) FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offer).Scan(&completed, &wrongAttempts); err != nil {
		t.Fatal(err)
	}
	if completed != events || wrongAttempts != 0 {
		t.Fatalf("completed=%d attempts_not_once=%d", completed, wrongAttempts)
	}
}
