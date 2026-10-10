package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/reservas/backend/internal/domain"
)

//go:embed schema.sql
var migration string

var ErrUnavailable = errors.New("reservation slot unavailable")
var ErrDateOutOfRange = errors.New("availability date outside booking window")
var ErrInvalidCapability = errors.New("reservation capability invalid")
var ErrInvalidState = errors.New("reservation state does not allow this operation")

type Reservation struct {
	ID               string    `json:"id"`
	ResourceID       string    `json:"resource_id"`
	StartsAt         time.Time `json:"starts_at"`
	EndsAt           time.Time `json:"ends_at"`
	LocalLabel       string    `json:"local_label"`
	TimeZone         string    `json:"time_zone"`
	AmountCents      int64     `json:"amount_cents"`
	ReservationState string    `json:"reservation_state"`
	PaymentState     string    `json:"payment_state"`
	ExpiresAt        time.Time `json:"expires_at"`
}

type CreatedReservation struct {
	Reservation
	Capability string `json:"capability"`
}

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open reservations postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping reservations postgres: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin reservations migration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(441208)`); err != nil {
		return fmt.Errorf("lock reservations migration: %w", err)
	}
	if _, err := tx.Exec(ctx, migration); err != nil {
		return fmt.Errorf("migrate reservations schema: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit reservations migration: %w", err)
	}
	return nil
}

func (s *Store) Availability(ctx context.Context, date string) ([]domain.Slot, domain.Resource, error) {
	resource, windows, err := s.schedule(ctx)
	if err != nil {
		return nil, domain.Resource{}, err
	}
	location, err := time.LoadLocation(resource.TimeZone)
	if err != nil {
		return nil, domain.Resource{}, fmt.Errorf("load resource timezone: %w", err)
	}
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil || parsedDate.Format("2006-01-02") != date {
		return nil, domain.Resource{}, fmt.Errorf("invalid local date")
	}
	localDay := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, location)
	var now time.Time
	if err := s.pool.QueryRow(ctx, `SELECT now()`).Scan(&now); err != nil {
		return nil, domain.Resource{}, fmt.Errorf("read database clock for availability: %w", err)
	}
	today := now.In(location)
	firstDate := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, location)
	if localDay.Before(firstDate) || localDay.After(firstDate.AddDate(0, 0, 14)) {
		return nil, domain.Resource{}, ErrDateOutOfRange
	}
	dayStart := localDay.UTC()
	dayEnd := localDay.AddDate(0, 0, 1).UTC()
	rows, err := s.pool.Query(ctx, `SELECT starts_at, occupied_until FROM reservations
		WHERE resource_id=$1 AND reservation_state IN ('held','confirmed') AND (reservation_state='confirmed' OR expires_at > $2)
		AND starts_at < $4 AND occupied_until > $3`, resource.ID, now, dayStart, dayEnd)
	if err != nil {
		return nil, domain.Resource{}, fmt.Errorf("query busy reservation intervals: %w", err)
	}
	busy := make([]domain.BusyInterval, 0)
	for rows.Next() {
		var interval domain.BusyInterval
		if err := rows.Scan(&interval.StartsAt, &interval.OccupiedUntil); err != nil {
			rows.Close()
			return nil, domain.Resource{}, fmt.Errorf("scan busy interval: %w", err)
		}
		busy = append(busy, interval)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, domain.Resource{}, fmt.Errorf("iterate busy intervals: %w", err)
	}
	rows.Close()
	slots, err := domain.AvailableSlots(resource, windows, date, now, busy)
	if err != nil {
		return nil, domain.Resource{}, err
	}
	return slots, resource, nil
}

func (s *Store) CreateHold(ctx context.Context, startsAt time.Time) (CreatedReservation, error) {
	const capabilityBytes = 32
	capabilityBytesValue := make([]byte, capabilityBytes)
	if _, err := rand.Read(capabilityBytesValue); err != nil {
		return CreatedReservation{}, fmt.Errorf("generate reservation capability: %w", err)
	}
	capability := hex.EncodeToString(capabilityBytesValue)
	capabilityHash := sha256.Sum256([]byte(capability))
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return CreatedReservation{}, fmt.Errorf("begin reservation hold: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	resource, windows, err := scheduleTx(ctx, tx)
	if err != nil {
		return CreatedReservation{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET reservation_state='expired', payment_state='expired'
		WHERE resource_id=$1 AND reservation_state='held' AND expires_at <= now()`, resource.ID); err != nil {
		return CreatedReservation{}, fmt.Errorf("expire stale holds before booking: %w", err)
	}
	location, err := time.LoadLocation(resource.TimeZone)
	if err != nil {
		return CreatedReservation{}, fmt.Errorf("load resource timezone for hold: %w", err)
	}
	var databaseNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&databaseNow); err != nil {
		return CreatedReservation{}, fmt.Errorf("read database clock for hold: %w", err)
	}
	localNow := databaseNow.In(location)
	localStart := startsAt.In(location)
	firstDate := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, location)
	requestedDate := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), 0, 0, 0, 0, location)
	if requestedDate.Before(firstDate) || requestedDate.After(firstDate.AddDate(0, 0, 14)) {
		return CreatedReservation{}, ErrUnavailable
	}
	localDate := localStart.Format("2006-01-02")
	slots, err := availabilityTx(ctx, tx, resource, windows, localDate, databaseNow)
	if err != nil {
		return CreatedReservation{}, err
	}
	var chosen *domain.Slot
	for index := range slots {
		if slots[index].StartsAt.Equal(startsAt.UTC()) {
			chosen = &slots[index]
			break
		}
	}
	if chosen == nil {
		return CreatedReservation{}, ErrUnavailable
	}
	id := uuid.New()
	paymentID := uuid.New()
	reservation := Reservation{
		ID: id.String(), ResourceID: resource.ID, StartsAt: chosen.StartsAt, EndsAt: chosen.EndsAt,
		LocalLabel: chosen.LocalLabel, TimeZone: chosen.TimeZone, AmountCents: resource.PriceCents,
		ReservationState: "held", PaymentState: "pending",
	}
	err = tx.QueryRow(ctx, `INSERT INTO reservations(id,resource_id,starts_at,ends_at,occupied_until,amount_cents,reservation_state,payment_state,expires_at,capability_hash)
		VALUES($1,$2,$3,$4,$5,$6,'held','pending',now()+($7 * interval '1 second'),$8) RETURNING expires_at`, id, resource.ID, reservation.StartsAt, reservation.EndsAt,
		reservation.EndsAt.Add(time.Duration(resource.BufferMinutes)*time.Minute), resource.PriceCents, resource.HoldSeconds, capabilityHash[:]).Scan(&reservation.ExpiresAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return CreatedReservation{}, ErrUnavailable
		}
		return CreatedReservation{}, fmt.Errorf("insert reservation hold: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO simulated_payments(id,reservation_id,amount_cents,payment_state) VALUES($1,$2,$3,'pending')`, paymentID, id, resource.PriceCents)
	if err != nil {
		return CreatedReservation{}, fmt.Errorf("create local simulated payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return CreatedReservation{}, fmt.Errorf("commit reservation hold outcome may be unknown: %w", err)
	}
	return CreatedReservation{Reservation: reservation, Capability: capability}, nil
}

func (s *Store) GetPrivate(ctx context.Context, id, capability string) (Reservation, error) {
	hash := sha256.Sum256([]byte(capability))
	reservation, err := scanReservation(s.pool.QueryRow(ctx, `SELECT r.id::text,r.resource_id,r.starts_at,r.ends_at,r.amount_cents,r.reservation_state,r.payment_state,r.expires_at,resource.time_zone
		FROM reservations r JOIN resources resource ON resource.id=r.resource_id WHERE r.id=$1 AND r.capability_hash=$2`, id, hash[:]))
	if err != nil {
		return Reservation{}, err
	}
	location, err := time.LoadLocation(reservation.TimeZone)
	if err != nil {
		return Reservation{}, fmt.Errorf("load reservation timezone: %w", err)
	}
	reservation.LocalLabel = reservation.StartsAt.In(location).Format("Mon 02 Jan 15:04 -07:00")
	return reservation, nil
}

func (s *Store) Cancel(ctx context.Context, id, capability string) (Reservation, error) {
	hash := sha256.Sum256([]byte(capability))
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, fmt.Errorf("begin reservation cancellation: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var state string
	err = tx.QueryRow(ctx, `SELECT reservation_state FROM reservations WHERE id=$1 AND capability_hash=$2 FOR UPDATE`, id, hash[:]).Scan(&state)
	if err != nil {
		return Reservation{}, err
	}
	if state != "held" {
		return Reservation{}, ErrInvalidState
	}
	_, err = tx.Exec(ctx, `UPDATE reservations SET reservation_state='cancelled',payment_state='cancelled' WHERE id=$1`, id)
	if err != nil {
		return Reservation{}, fmt.Errorf("cancel held reservation: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE simulated_payments SET payment_state='cancelled',updated_at=now() WHERE reservation_id=$1 AND payment_state='pending'`, id)
	if err != nil {
		return Reservation{}, fmt.Errorf("cancel simulated payment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, fmt.Errorf("commit reservation cancellation: %w", err)
	}
	return s.GetPrivate(ctx, id, capability)
}

func (s *Store) SimulatePaid(ctx context.Context, id, eventKey string) error {
	if len(eventKey) < 16 || len(eventKey) > 128 {
		return fmt.Errorf("event key must contain 16 to 128 characters")
	}
	eventID := uuid.New()
	_, err := s.pool.Exec(ctx, `INSERT INTO payment_events(id,reservation_id,event_key,event_type)
		SELECT $1,id,$3,'paid' FROM reservations WHERE id=$2 ON CONFLICT(event_key) DO NOTHING`, eventID, id, eventKey)
	if err != nil {
		return fmt.Errorf("persist local payment event: %w", err)
	}
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM payment_events WHERE event_key=$1 AND reservation_id=$2)`, eventKey, id).Scan(&exists); err != nil {
		return fmt.Errorf("check local payment event idempotency: %w", err)
	}
	if !exists {
		return ErrUnavailable
	}
	return nil
}

func (s *Store) ExpireHolds(ctx context.Context, limit int) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin hold expiration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	command, err := tx.Exec(ctx, `WITH expired AS (
		SELECT id FROM reservations WHERE reservation_state='held' AND expires_at <= now()
		ORDER BY expires_at,id FOR UPDATE SKIP LOCKED LIMIT $1
	) UPDATE reservations r SET reservation_state='expired',payment_state='expired' FROM expired WHERE r.id=expired.id`, limit)
	if err != nil {
		return 0, fmt.Errorf("expire reservation holds: %w", err)
	}
	_, err = tx.Exec(ctx, `UPDATE simulated_payments SET payment_state='expired',updated_at=now()
		WHERE payment_state='pending' AND reservation_id IN (SELECT id FROM reservations WHERE reservation_state='expired' AND expires_at<=now())`)
	if err != nil {
		return 0, fmt.Errorf("expire simulated payments: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit hold expiration: %w", err)
	}
	return command.RowsAffected(), nil
}

func (s *Store) ProcessPaymentEvents(ctx context.Context, limit int) (int64, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return 0, fmt.Errorf("begin reservation event worker: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	rows, err := tx.Query(ctx, `SELECT e.id,e.reservation_id,r.expires_at FROM payment_events e
		JOIN reservations r ON r.id=e.reservation_id
		WHERE e.processed_at IS NULL AND (e.lease_until IS NULL OR e.lease_until < now())
		ORDER BY e.received_at,e.id FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return 0, fmt.Errorf("claim reservation payment events: %w", err)
	}
	type claimedEvent struct {
		eventID       uuid.UUID
		reservationID uuid.UUID
		expiresAt     time.Time
	}
	events := make([]claimedEvent, 0)
	for rows.Next() {
		var event claimedEvent
		if err := rows.Scan(&event.eventID, &event.reservationID, &event.expiresAt); err != nil {
			rows.Close()
			return 0, fmt.Errorf("scan reservation payment event: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, fmt.Errorf("iterate reservation payment events: %w", err)
	}
	rows.Close()
	var databaseNow time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&databaseNow); err != nil {
		return 0, fmt.Errorf("read database clock for payment events: %w", err)
	}
	for _, event := range events {
		var reservationState string
		var paymentState string
		if err := tx.QueryRow(ctx, `SELECT reservation_state,payment_state FROM reservations WHERE id=$1 FOR UPDATE`, event.reservationID).Scan(&reservationState, &paymentState); err != nil {
			return 0, fmt.Errorf("load payment target reservation: %w", err)
		}
		switch {
		case reservationState == "held" && paymentState == "pending" && event.expiresAt.After(databaseNow):
			_, err = tx.Exec(ctx, `UPDATE reservations SET reservation_state='confirmed',payment_state='paid' WHERE id=$1`, event.reservationID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE simulated_payments SET payment_state='paid',updated_at=now() WHERE reservation_id=$1`, event.reservationID)
			}
		case reservationState == "held" && paymentState == "pending":
			_, err = tx.Exec(ctx, `UPDATE reservations SET reservation_state='expired',payment_state='payment_exception' WHERE id=$1`, event.reservationID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE simulated_payments SET payment_state='payment_exception',updated_at=now() WHERE reservation_id=$1`, event.reservationID)
			}
		case reservationState == "confirmed" && paymentState == "paid":
			// Duplicate paid notifications are idempotent.
		case reservationState == "expired" || reservationState == "cancelled":
			_, err = tx.Exec(ctx, `UPDATE reservations SET payment_state='payment_exception' WHERE id=$1`, event.reservationID)
			if err == nil {
				_, err = tx.Exec(ctx, `UPDATE simulated_payments SET payment_state='payment_exception',updated_at=now() WHERE reservation_id=$1`, event.reservationID)
			}
		default:
			err = ErrInvalidState
		}
		if err != nil {
			return 0, fmt.Errorf("apply reservation payment event: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE payment_events SET processed_at=now(),lease_until=NULL,attempts=attempts+1,last_error=NULL WHERE id=$1`, event.eventID); err != nil {
			return 0, fmt.Errorf("ack reservation payment event: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, fmt.Errorf("commit reservation payment events: %w", err)
	}
	return int64(len(events)), nil
}

func (s *Store) Complete(ctx context.Context, id string) error {
	command, err := s.pool.Exec(ctx, `UPDATE reservations SET reservation_state='completed',completed_at=now()
		WHERE id=$1 AND reservation_state='confirmed' AND payment_state='paid' AND ends_at<=now()`, id)
	if err != nil {
		return fmt.Errorf("complete paid reservation: %w", err)
	}
	if command.RowsAffected() != 1 {
		return ErrInvalidState
	}
	return nil
}

func (s *Store) ListOperator(ctx context.Context) ([]Reservation, error) {
	rows, err := s.pool.Query(ctx, `SELECT r.id::text,r.resource_id,r.starts_at,r.ends_at,r.amount_cents,r.reservation_state,r.payment_state,r.expires_at,resource.time_zone
		FROM reservations r JOIN resources resource ON resource.id=r.resource_id
		WHERE r.reservation_state IN ('confirmed','completed','payment_exception') ORDER BY r.starts_at DESC LIMIT 200`)
	if err != nil {
		return nil, fmt.Errorf("query operator reservation list: %w", err)
	}
	defer rows.Close()
	reservations := make([]Reservation, 0)
	for rows.Next() {
		reservation, err := scanReservation(rows)
		if err != nil {
			return nil, fmt.Errorf("scan operator reservation: %w", err)
		}
		location, err := time.LoadLocation(reservation.TimeZone)
		if err != nil {
			return nil, fmt.Errorf("load operator reservation timezone: %w", err)
		}
		reservation.LocalLabel = reservation.StartsAt.In(location).Format("Mon 02 Jan 15:04 -07:00")
		reservations = append(reservations, reservation)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate operator reservations: %w", err)
	}
	return reservations, nil
}

func scanReservation(row pgx.Row) (Reservation, error) {
	var reservation Reservation
	err := row.Scan(&reservation.ID, &reservation.ResourceID, &reservation.StartsAt, &reservation.EndsAt,
		&reservation.AmountCents, &reservation.ReservationState, &reservation.PaymentState, &reservation.ExpiresAt, &reservation.TimeZone)
	return reservation, err
}

func (s *Store) schedule(ctx context.Context) (domain.Resource, []domain.OpeningWindow, error) {
	return scheduleTx(ctx, s.pool)
}

type queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func scheduleTx(ctx context.Context, q queryer) (domain.Resource, []domain.OpeningWindow, error) {
	var resource domain.Resource
	err := q.QueryRow(ctx, `SELECT id,name,time_zone,price_cents,duration_minutes,buffer_minutes,slot_increment_minutes,hold_seconds FROM resources WHERE id='demo-room'`).Scan(
		&resource.ID, &resource.Name, &resource.TimeZone, &resource.PriceCents, &resource.DurationMinutes,
		&resource.BufferMinutes, &resource.SlotIncrementMinutes, &resource.HoldSeconds)
	if err != nil {
		return domain.Resource{}, nil, fmt.Errorf("load reservation resource: %w", err)
	}
	rows, err := q.Query(ctx, `SELECT weekday, to_char(opens_at,'HH24:MI'), to_char(closes_at,'HH24:MI') FROM opening_windows WHERE resource_id=$1 ORDER BY weekday`, resource.ID)
	if err != nil {
		return domain.Resource{}, nil, fmt.Errorf("load opening windows: %w", err)
	}
	windows := make([]domain.OpeningWindow, 0)
	for rows.Next() {
		var weekday int
		var window domain.OpeningWindow
		if err := rows.Scan(&weekday, &window.OpensAt, &window.ClosesAt); err != nil {
			rows.Close()
			return domain.Resource{}, nil, fmt.Errorf("scan opening window: %w", err)
		}
		window.Weekday = time.Weekday(weekday)
		windows = append(windows, window)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return domain.Resource{}, nil, fmt.Errorf("iterate opening windows: %w", err)
	}
	rows.Close()
	return resource, windows, nil
}

func availabilityTx(ctx context.Context, tx pgx.Tx, resource domain.Resource, windows []domain.OpeningWindow, date string, now time.Time) ([]domain.Slot, error) {
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil || parsedDate.Format("2006-01-02") != date {
		return nil, fmt.Errorf("invalid local date")
	}
	location, err := time.LoadLocation(resource.TimeZone)
	if err != nil {
		return nil, fmt.Errorf("load resource timezone for slots: %w", err)
	}
	localDay := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, location)
	dayStart := localDay.UTC()
	dayEnd := localDay.AddDate(0, 0, 1).UTC()
	rows, err := tx.Query(ctx, `SELECT starts_at,occupied_until FROM reservations WHERE resource_id=$1 AND reservation_state IN ('held','confirmed')
		AND (reservation_state='confirmed' OR expires_at>$2) AND starts_at < $4 AND occupied_until > $3`, resource.ID, now, dayStart, dayEnd)
	if err != nil {
		return nil, fmt.Errorf("query intervals for hold: %w", err)
	}
	busy := make([]domain.BusyInterval, 0)
	for rows.Next() {
		var interval domain.BusyInterval
		if err := rows.Scan(&interval.StartsAt, &interval.OccupiedUntil); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan interval for hold: %w", err)
		}
		busy = append(busy, interval)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate intervals for hold: %w", err)
	}
	rows.Close()
	return domain.AvailableSlots(resource, windows, date, now, busy)
}
