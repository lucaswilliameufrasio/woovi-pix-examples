package httpapi

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func TestOrderRoutesAgainstPostgres(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	lockID := int64(421337)
	lockConn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lockConn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockID); err != nil {
		lockConn.Release()
		t.Fatal(err)
	}
	defer func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockID)
		lockConn.Release()
	}()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	offerID := "http-" + testUnique(t)
	orderID := ""
	if _, err := pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'HTTP integration',1250,1,1,60)`, offerID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
	}()
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE offer_id=$1`, offerID) }()
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE offers SET available_units=total_units WHERE id=$1`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM offers WHERE id=$1`, offerID)
	}()
	handler := (API{Store: st}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`"}`))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created store.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	orderID = created.ID
	var persisted int64
	if err := pool.QueryRow(ctx, `SELECT amount_cents FROM orders WHERE id=$1`, orderID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != 1250 {
		t.Fatalf("persisted amount=%d", persisted)
	}
	req = httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`","amount_cents":1}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown client price field status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`"}`))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("oversell status=%d", rec.Code)
	}
	webhookBody := `{"order_id":"` + orderID + `","event_key":"http-paid-` + testUnique(t) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/dev/webhooks/paid", bytes.NewBufferString(webhookBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("webhook status=%d body=%s", rec.Code, rec.Body.String())
	}
	var events int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_events WHERE order_id=$1`, orderID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("persisted webhook events=%d", events)
	}
	req = httptest.NewRequest(http.MethodPost, "/dev/webhooks/paid", bytes.NewBufferString(webhookBody))
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("duplicate webhook status=%d", rec.Code)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_events WHERE order_id=$1`, orderID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("duplicate persisted event count=%d", events)
	}
}

func testUnique(t *testing.T) string {
	t.Helper()
	var x [8]byte
	if _, err := rand.Read(x[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(x[:])
}
