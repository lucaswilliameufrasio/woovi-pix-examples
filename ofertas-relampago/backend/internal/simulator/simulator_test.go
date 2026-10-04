package simulator

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/httpapi"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func TestLocalPaymentSimulatorEndToEnd(t *testing.T) {
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
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	offerID := "sim-" + hex.EncodeToString(suffix[:])
	if _, err := pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Simulator E2E',1999,2,2,60)`, offerID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM simulated_charges WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE offer_id=$1`, offerID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM offers WHERE id=$1`, offerID)
	}()
	api := httptest.NewServer((httpapi.API{Store: st}).Handler())
	defer api.Close()
	apiHandler := (httpapi.API{Store: st}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewBufferString(`{"offer_id":"`+offerID+`"}`))
	rec := httptest.NewRecorder()
	apiHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create order status=%d body=%s", rec.Code, rec.Body.String())
	}
	var order store.Order
	if err := json.Unmarshal(rec.Body.Bytes(), &order); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{}
	lookup := func(id string) (int64, error) { o, err := st.GetOrder(ctx, id); return o.Amount, err }
	deliver := func(id, eventKey string) error {
		body, _ := json.Marshal(map[string]string{"order_id": id, "event_key": eventKey})
		resp, err := client.Post(api.URL+"/dev/webhooks/paid", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("webhook HTTP status %d", resp.StatusCode)
		}
		return nil
	}
	sim := httptest.NewServer(New(st, lookup, deliver).Handler())
	defer sim.Close()
	chargeBody, _ := json.Marshal(map[string]string{"order_id": order.ID})
	resp, err := client.Post(sim.URL+"/charges", "application/json", bytes.NewReader(chargeBody))
	if err != nil {
		t.Fatal(err)
	}
	var charge Charge
	if err := json.NewDecoder(resp.Body).Decode(&charge); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || charge.AmountCents != 1999 {
		t.Fatalf("charge=%+v status=%d", charge, resp.StatusCode)
	}
	// The timeout scenario persists a charge but drops the response; retrying
	// with the same order reference must recover the exact same charge.
	other, err := st.CreateOrder(ctx, "timeout-"+offerID, offerID)
	if err != nil {
		t.Fatal(err)
	}
	resp, err = client.Post(sim.URL+"/dev/scenarios/timeout?order_id="+other.ID, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusGatewayTimeout {
		t.Fatalf("timeout scenario status=%d", resp.StatusCode)
	}
	resp, err = client.Post(sim.URL+"/charges/"+other.ID+"/retry", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	var retried Charge
	if err := json.NewDecoder(resp.Body).Decode(&retried); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || retried.ID != "sim-"+other.ID || retried.AmountCents != other.Amount {
		t.Fatalf("retry charge=%+v status=%d", retried, resp.StatusCode)
	}
	sim.Close()
	// A fresh simulator process reads the same durable charge from PostgreSQL.
	sim = httptest.NewServer(New(st, lookup, deliver).Handler())
	defer sim.Close()
	resp, err = client.Get(sim.URL + "/charges/" + order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var recovered Charge
	if err := json.NewDecoder(resp.Body).Decode(&recovered); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || recovered.ID != charge.ID || recovered.AmountCents != charge.AmountCents {
		t.Fatalf("recovered charge=%+v status=%d", recovered, resp.StatusCode)
	}
	resp, err = client.Post(sim.URL+"/charges/"+order.ID+"/pay", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pay status=%d", resp.StatusCode)
	}
	resp, err = client.Post(sim.URL+"/dev/scenarios/duplicate-paid/"+order.ID, "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("duplicate webhook scenario status=%d", resp.StatusCode)
	}
	resp, err = client.Get(sim.URL + "/charges/" + order.ID)
	if err != nil {
		t.Fatal(err)
	}
	var paidCharge Charge
	if err := json.NewDecoder(resp.Body).Decode(&paidCharge); err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if paidCharge.Status != "paid" {
		t.Fatalf("charge after pay status=%s", paidCharge.Status)
	}
	if _, err := st.ProcessPaymentEvents(ctx, 10); err != nil {
		t.Fatal(err)
	}
	paid, err := st.GetOrder(ctx, order.ID)
	if err != nil {
		t.Fatal(err)
	}
	if paid.State != "paid" {
		t.Fatalf("authoritative order state=%s", paid.State)
	}
}

func TestDevelopmentScenarioRoutesRequireDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	h := New(nil, nil, nil).Handler()
	for _, path := range []string{
		"/dev/scenarios/timeout?order_id=order",
		"/dev/scenarios/duplicate-paid/order",
		"/dev/webhooks/order/late-paid",
	} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("POST %s status=%d, want 404", path, recorder.Code)
		}
	}
}

func TestRetryChargeCannotUseAnotherOrderReference(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	st, err := store.Open(context.Background(), os.Getenv("TEST_DATABASE_URL"))
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	lookup := func(id string) (int64, error) {
		order, err := st.GetOrder(context.Background(), id)
		return order.Amount, err
	}
	h := New(st, lookup, nil).Handler()
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/charges/order-a/retry", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("retry without existing order status=%d, want 404", recorder.Code)
	}
}

func TestAllPaymentMutationRoutesAreHiddenWithoutDemoMode(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	h := New(nil, nil, nil).Handler()
	for _, path := range []string{
		"/charges/order-a/pay",
		"/charges/order-a/retry",
		"/dev/webhooks/order-a/paid",
		"/dev/webhooks/order-a/late-paid",
		"/dev/scenarios/timeout?order_id=order-a",
		"/dev/scenarios/duplicate-paid/order-a",
	} {
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Errorf("POST %s status=%d, want 404", path, recorder.Code)
		}
	}
}

func TestPayEndpointDoesNotClaimOperationalSuccessForLatePayment(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	offer := "late-sim-" + testSimulatorID(t)
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Late payment test',500,1,1,60)`, offer); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM simulated_charges WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offer)
		_, _ = pool.Exec(context.Background(), `DELETE FROM payment_events WHERE order_id IN (SELECT id FROM orders WHERE offer_id=$1)`, offer)
		_, _ = pool.Exec(context.Background(), `DELETE FROM orders WHERE offer_id=$1`, offer)
		_, _ = pool.Exec(context.Background(), `DELETE FROM offers WHERE id=$1`, offer)
	}()
	order, err := st.CreateOrder(ctx, "late-order-"+offer, offer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateSimulatedCharge(ctx, order.ID, order.Amount); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, order.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ExpireOrders(ctx); err != nil {
		t.Fatal(err)
	}
	h := New(st, func(id string) (int64, error) { o, err := st.GetOrder(ctx, id); return o.Amount, err }, func(id, key string) error {
		_, err := st.PersistPaidEvent(ctx, key, id, key)
		return err
	}).Handler()
	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/charges/"+order.ID+"/pay", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "late payment recorded for review") {
		t.Fatalf("late pay status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	charge, err := st.GetSimulatedCharge(ctx, order.ID)
	if err != nil || charge.Status != "paid" {
		t.Fatalf("charge=%+v err=%v", charge, err)
	}
	if _, err := st.ProcessPaymentEvents(ctx, 10); err != nil {
		t.Fatal(err)
	}
	final, err := st.GetOrder(ctx, order.ID)
	if err != nil || final.State != "payment_exception" {
		t.Fatalf("order=%+v err=%v", final, err)
	}
}

func testSimulatorID(t *testing.T) string {
	t.Helper()
	var id [8]byte
	if _, err := rand.Read(id[:]); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(id[:])
}
