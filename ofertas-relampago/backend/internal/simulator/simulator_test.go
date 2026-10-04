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
	if _, err := pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Simulator E2E',1999,1,1,60)`, offerID); err != nil {
		t.Fatal(err)
	}
	defer func() {
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
	sim := httptest.NewServer(New(lookup, deliver).Handler())
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
	resp, err = client.Post(sim.URL+"/charges/"+order.ID+"/pay", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pay status=%d", resp.StatusCode)
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
