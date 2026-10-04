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
	"strings"
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
		_, _ = pool.Exec(context.Background(), `DELETE FROM simulated_charges WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
	}()
	defer func() { _, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE offer_id=$1`, offerID) }()
	defer func() {
		_, _ = pool.Exec(context.Background(), `UPDATE offers SET available_units=total_units WHERE id=$1`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM offers WHERE id=$1`, offerID)
	}()
	handler := (API{Store: st}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`"}`))
	req.Header.Set("Content-Type", "application/json")
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
	req = httptest.NewRequest(http.MethodGet, "/v1/orders/"+orderID, nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("get status=%d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`","amount_cents":1}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error_code":"MALFORMED_REQUEST"`) || !strings.Contains(rec.Body.String(), `"message":`) {
		t.Fatalf("unknown client price field status=%d", rec.Code)
	}
	largeBody := `{"offer_id":"` + strings.Repeat("x", 5000) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(largeBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"error_code":"PAYLOAD_TOO_LARGE"`) {
		t.Fatalf("oversized body status=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(`{"offer_id":""}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"error_code":"INVALID_PARAMS"`) || !strings.Contains(rec.Body.String(), `"validation_errors"`) {
		t.Fatalf("semantic validation status=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(`{"offer_id":`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"error_code":"MALFORMED_REQUEST"`) {
		t.Fatalf("malformed JSON status=%d body=%s", rec.Code, rec.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("oversell status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"error_code":"OFFER_UNAVAILABLE"`) || !strings.Contains(rec.Body.String(), `"message":`) {
		t.Fatalf("conflict does not follow house error contract: %s", rec.Body.String())
	}
	webhookBody := `{"order_id":"` + orderID + `","event_key":"http-paid-` + testUnique(t) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/dev/webhooks/paid", bytes.NewBufferString(webhookBody))
	req.Header.Set("Content-Type", "application/json")
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
	req = httptest.NewRequest(http.MethodPost, "/v1/dev/webhooks/paid", bytes.NewBufferString(webhookBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("duplicate webhook status=%d", rec.Code)
	}
	legacy := httptest.NewRecorder()
	handler.ServeHTTP(legacy, httptest.NewRequest(http.MethodGet, "/orders/"+orderID, nil))
	if legacy.Code != http.StatusNotFound {
		t.Fatalf("unversioned API alias status=%d, want 404", legacy.Code)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM payment_events WHERE order_id=$1`, orderID).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 1 {
		t.Fatalf("duplicate persisted event count=%d", events)
	}
}

func TestHTTPErrorBuilderContract(t *testing.T) {
	tests := []struct {
		status     int
		code       string
		wantStatus int
		wantCode   string
	}{
		{http.StatusBadRequest, "MALFORMED_REQUEST", 400, "MALFORMED_REQUEST"},
		{http.StatusUnprocessableEntity, "INVALID_PARAMS", 422, "INVALID_PARAMS"},
		{http.StatusNotFound, "ORDER_NOT_FOUND", 404, "ORDER_NOT_FOUND"},
		{http.StatusConflict, "IDEMPOTENCY_CONFLICT", 409, "IDEMPOTENCY_CONFLICT"},
		{http.StatusBadGateway, "DEPENDENCY_REQUEST", 502, "DEPENDENCY_REQUEST"},
		{http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", 413, "PAYLOAD_TOO_LARGE"},
		{http.StatusInternalServerError, "db details must not leak", 500, "UNEXPECTED_ERROR"},
	}
	for _, tc := range tests {
		t.Run(tc.wantCode, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			writeError(recorder, tc.status, tc.code)
			var body map[string]any
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != tc.wantStatus || body["error_code"] != tc.wantCode || body["message"] == nil {
				t.Fatalf("status=%d body=%v", recorder.Code, body)
			}
			if strings.Contains(recorder.Body.String(), "db details") {
				t.Fatal("internal detail leaked")
			}
		})
	}
}

func TestAPIOnlyRegistersVersionedRoutesAndDevRouteIsModeGated(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	h := (API{}).Handler()
	for _, path := range []string{"/offers", "/orders", "/healthz", "/v1/dev/webhooks/paid"} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("legacy/disabled route %s status=%d", path, recorder.Code)
		}
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
