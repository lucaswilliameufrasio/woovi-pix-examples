package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationStore(t *testing.T) *Store {
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
	schema := fmt.Sprintf("reservations_%d", time.Now().UnixNano())
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	st := &Store{pool: pool}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return st
}

func futureWeekdayAtNine(t *testing.T) time.Time {
	t.Helper()
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, 1)
	for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		date = date.AddDate(0, 0, 1)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, location).UTC()
}

func TestConcurrentHoldsCannotOverlapAndAdjacentBufferBoundaryIsAllowed(t *testing.T) {
	s := integrationStore(t)
	startAt := futureWeekdayAtNine(t)
	const clients = 20
	start := make(chan struct{})
	var group sync.WaitGroup
	var mutex sync.Mutex
	created := 0
	unavailable := 0
	var failures []error
	for range clients {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			_, err := s.CreateHold(context.Background(), startAt)
			mutex.Lock()
			defer mutex.Unlock()
			if err == nil {
				created++
				return
			}
			if errors.Is(err, ErrUnavailable) {
				unavailable++
				return
			}
			failures = append(failures, err)
		}()
	}
	close(start)
	group.Wait()
	if len(failures) != 0 || created != 1 || unavailable != clients-1 {
		t.Fatalf("created=%d unavailable=%d failures=%v", created, unavailable, failures)
	}
	if _, err := s.CreateHold(context.Background(), startAt.Add(45*time.Minute)); err != nil {
		t.Fatalf("adjacent appointment after buffer should be allowed: %v", err)
	}
	if _, err := s.CreateHold(context.Background(), startAt.Add(30*time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("overlapping appointment must be unavailable, got %v", err)
	}
}

func TestPaymentAfterExpiredHoldBecomesExceptionWithoutReacquiringSlot(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	startAt := futureWeekdayAtNine(t)
	created, err := s.CreateHold(ctx, startAt)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE reservations SET expires_at=now()-interval '1 second' WHERE id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if count, err := s.ExpireHolds(ctx, 50); err != nil || count != 1 {
		t.Fatalf("expired=%d error=%v", count, err)
	}
	if err := s.SimulatePaid(ctx, created.ID, "late-event-key-0001"); err != nil {
		t.Fatal(err)
	}
	if count, err := s.ProcessPaymentEvents(ctx, 50); err != nil || count != 1 {
		t.Fatalf("processed=%d error=%v", count, err)
	}
	reservation, err := s.GetPrivate(ctx, created.ID, created.Capability)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.ReservationState != "expired" || reservation.PaymentState != "payment_exception" {
		t.Fatalf("unexpected late payment states: reservation=%s payment=%s", reservation.ReservationState, reservation.PaymentState)
	}
	if _, err := s.CreateHold(ctx, startAt); err != nil {
		t.Fatalf("late payment must not reacquire released slot: %v", err)
	}
}

func TestPaymentEventProcessedAfterHoldDeadlineIsLateEvenBeforeExpiryWorker(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	created, err := s.CreateHold(ctx, futureWeekdayAtNine(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, `UPDATE reservations SET expires_at=now()-interval '1 second' WHERE id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.SimulatePaid(ctx, created.ID, "deadline-event-key-0001"); err != nil {
		t.Fatal(err)
	}
	processed, err := s.ProcessPaymentEvents(ctx, 20)
	if err != nil || processed != 1 {
		t.Fatalf("processed=%d error=%v", processed, err)
	}
	reservation, err := s.GetPrivate(ctx, created.ID, created.Capability)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.ReservationState != "expired" || reservation.PaymentState != "payment_exception" {
		t.Fatalf("event after deadline was incorrectly accepted: reservation=%s payment=%s", reservation.ReservationState, reservation.PaymentState)
	}
}

func TestReservationCapabilityAndCancellationArePrivate(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	created, err := s.CreateHold(ctx, futureWeekdayAtNine(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetPrivate(ctx, created.ID, "wrong-capability"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("wrong capability should not reveal reservation: %v", err)
	}
	cancelled, err := s.Cancel(ctx, created.ID, created.Capability)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.ReservationState != "cancelled" || cancelled.PaymentState != "cancelled" {
		t.Fatalf("unexpected cancellation states: %+v", cancelled)
	}
}

func TestPendingPaymentEventSurvivesStorePoolRestart(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	created, err := s.CreateHold(ctx, futureWeekdayAtNine(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SimulatePaid(ctx, created.ID, "restart-event-key-0001"); err != nil {
		t.Fatal(err)
	}
	poolConfig := s.pool.Config()
	s.Close()

	restartedPool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Store{pool: restartedPool}
	t.Cleanup(restarted.Close)
	processed, err := restarted.ProcessPaymentEvents(ctx, 20)
	if err != nil || processed != 1 {
		t.Fatalf("processed after pool restart=%d error=%v", processed, err)
	}
	reservation, err := restarted.GetPrivate(ctx, created.ID, created.Capability)
	if err != nil {
		t.Fatal(err)
	}
	if reservation.ReservationState != "confirmed" || reservation.PaymentState != "paid" {
		t.Fatalf("payment event was not recovered after restart: %+v", reservation)
	}
}

func TestMigrationCanRepeatWithoutDroppingTheOverlapConstraint(t *testing.T) {
	s := integrationStore(t)
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var constraintCount int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE conname='reservations_no_overlap' AND conrelid='reservations'::regclass`).Scan(&constraintCount); err != nil {
		t.Fatal(err)
	}
	if constraintCount != 1 {
		t.Fatalf("expected exactly one overlap constraint after repeat migration, got %d", constraintCount)
	}
	startAt := futureWeekdayAtNine(t)
	if _, err := s.CreateHold(ctx, startAt); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateHold(ctx, startAt.Add(30*time.Minute)); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("overlap constraint stopped protecting after repeat migration: %v", err)
	}
}
