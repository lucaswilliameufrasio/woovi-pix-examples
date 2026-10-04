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
	operatorToken := "local-test-operator-token-0123456789abcdef"
	t.Setenv("DEMO_OPERATOR_TOKEN", operatorToken)
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
	missingOfferReq := httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(`{"offer_id":"missing-`+testUnique(t)+`"}`))
	missingOfferReq.Header.Set("Content-Type", "application/json")
	missingOfferRec := httptest.NewRecorder()
	handler.ServeHTTP(missingOfferRec, missingOfferReq)
	if missingOfferRec.Code != http.StatusNotFound || !strings.Contains(missingOfferRec.Body.String(), `"error_code":"OFFER_NOT_FOUND"`) {
		t.Fatalf("missing offer status=%d body=%s", missingOfferRec.Code, missingOfferRec.Body.String())
	}
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
	invalidOffer := `{"offer_id":"` + strings.Repeat("x", 201) + `"}`
	req = httptest.NewRequest(http.MethodPost, "/v1/orders", strings.NewReader(invalidOffer))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `"error_code":"INVALID_PARAMS"`) || !strings.Contains(rec.Body.String(), `"field":"offer_id"`) {
		t.Fatalf("invalid resource identifier status=%d body=%s", rec.Code, rec.Body.String())
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
	if rec.Code != http.StatusPreconditionFailed {
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
	paidRequest := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/"+orderID+"/paid", nil)
	paidRequest.Header.Set("Authorization", "Bearer "+operatorToken)
	paidResponse := httptest.NewRecorder()
	handler.ServeHTTP(paidResponse, paidRequest)
	if paidResponse.Code != http.StatusOK {
		t.Fatalf("mark paid status=%d body=%s", paidResponse.Code, paidResponse.Body.String())
	}
	paidResponse = httptest.NewRecorder()
	handler.ServeHTTP(paidResponse, paidRequest)
	if paidResponse.Code != http.StatusConflict || !strings.Contains(paidResponse.Body.String(), `"error_code":"ORDER_NOT_ELIGIBLE"`) || !strings.Contains(paidResponse.Body.String(), `"order_state":"paid"`) {
		t.Fatalf("repeat paid transition status=%d body=%s", paidResponse.Code, paidResponse.Body.String())
	}
	issueRequest := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/"+orderID+"/pickup-token", nil)
	issueRequest.Header.Set("Authorization", "Bearer "+operatorToken)
	issueResponse := httptest.NewRecorder()
	handler.ServeHTTP(issueResponse, issueRequest)
	var tokenBody map[string]string
	if err := json.Unmarshal(issueResponse.Body.Bytes(), &tokenBody); err != nil {
		t.Fatal(err)
	}
	if issueResponse.Code != http.StatusCreated || tokenBody["pickup_token"] == "" {
		t.Fatalf("issue pickup token status=%d body=%s", issueResponse.Code, issueResponse.Body.String())
	}
	pickupPayload, _ := json.Marshal(map[string]string{"pickup_token": tokenBody["pickup_token"]})
	pickupRequest := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/"+orderID+"/pickup", bytes.NewReader(pickupPayload))
	pickupRequest.Header.Set("Content-Type", "application/json")
	pickupRequest.Header.Set("Authorization", "Bearer "+operatorToken)
	pickupResponse := httptest.NewRecorder()
	handler.ServeHTTP(pickupResponse, pickupRequest)
	if pickupResponse.Code != http.StatusOK || !strings.Contains(pickupResponse.Body.String(), `"picked_up_at"`) {
		t.Fatalf("pickup status=%d body=%s", pickupResponse.Code, pickupResponse.Body.String())
	}
	duplicatePickupRequest := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/"+orderID+"/pickup", bytes.NewReader(pickupPayload))
	duplicatePickupRequest.Header.Set("Content-Type", "application/json")
	duplicatePickupRequest.Header.Set("Authorization", "Bearer "+operatorToken)
	pickupResponse = httptest.NewRecorder()
	handler.ServeHTTP(pickupResponse, duplicatePickupRequest)
	if pickupResponse.Code != http.StatusConflict || !strings.Contains(pickupResponse.Body.String(), `"error_code":"PICKUP_ALREADY_DONE"`) {
		t.Fatalf("duplicate pickup status=%d body=%s", pickupResponse.Code, pickupResponse.Body.String())
	}
}

func TestOperatorRoutesRequireConfiguredBearerToken(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("DEMO_OPERATOR_TOKEN", "operator-secret-at-least-32-characters")
	handler := (API{}).Handler()
	for _, tc := range []struct {
		authorization string
		status        int
	}{
		{"", http.StatusUnauthorized},
		{"Bearer wrong-token", http.StatusUnauthorized},
	} {
		req := httptest.NewRequest(http.MethodGet, "/v1/operator/orders", nil)
		if tc.authorization != "" {
			req.Header.Set("Authorization", tc.authorization)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.status {
			t.Errorf("authorization=%q status=%d want=%d body=%s", tc.authorization, rec.Code, tc.status, rec.Body.String())
		}
		if tc.status == http.StatusUnauthorized && !strings.Contains(rec.Body.String(), `"error_code":"OPERATOR_UNAUTHORIZED"`) {
			t.Errorf("missing house auth error: %s", rec.Body.String())
		}
	}
	t.Setenv("DEMO_OPERATOR_TOKEN", "short")
	req := httptest.NewRequest(http.MethodGet, "/v1/operator/orders", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), `"error_code":"UNEXPECTED_ERROR"`) {
		t.Fatalf("missing operator secret status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPreconditionErrorIncludesExtraAndCorrectStatus(t *testing.T) {
	recorder := httptest.NewRecorder()
	writePreconditionError(recorder, "ORDER_NOT_ELIGIBLE", map[string]any{"order_state": "expired"})
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	extra, ok := body["extra"].(map[string]any)
	if recorder.Code != http.StatusPreconditionFailed || body["error_code"] != "ORDER_NOT_ELIGIBLE" || !ok || extra["order_state"] != "expired" {
		t.Fatalf("status=%d body=%v", recorder.Code, body)
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
		{http.StatusNotFound, "OFFER_NOT_FOUND", 404, "OFFER_NOT_FOUND"},
		{http.StatusConflict, "IDEMPOTENCY_CONFLICT", 409, "IDEMPOTENCY_CONFLICT"},
		{http.StatusConflict, "OFFER_UNAVAILABLE", 409, "OFFER_UNAVAILABLE"},
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
	for _, path := range []string{"/v1/operator/orders/order/pickup-token", "/v1/operator/orders/order/pickup"} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"pickup_token":"x"}`)))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("operator route outside demo %s status=%d", path, recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/v1/operator/orders", nil))
	if recorder.Code != http.StatusNotFound {
		t.Errorf("operator list outside demo status=%d", recorder.Code)
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
