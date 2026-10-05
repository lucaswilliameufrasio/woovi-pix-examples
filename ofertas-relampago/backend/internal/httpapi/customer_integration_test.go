package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func TestCustomerOrderCapabilityCreationAndScope(t *testing.T) {
	f := newCheckoutFixture(t)
	r := f.requireResponse(t, http.MethodPost, "/v1/orders", "", "", `{"offer_id":"demo-offer"}`, 201, "")
	var created store.OrderAccess
	if err := json.Unmarshal(r.body, &created); err != nil {
		t.Fatal(err)
	}
	if len(created.AccessToken) != 64 || created.Amount != 2500 || !created.TokenExpiresAt.After(time.Now()) || created.TokenExpiresAt.After(time.Now().Add(20*time.Minute+time.Second)) {
		t.Fatal("reservation capability/TTL/price invalid (values omitted)")
	}
	if r.header.Get("Cache-Control") != "no-store" {
		t.Fatal("creation credential response is cacheable")
	}
	path := "/v1/customer/orders/" + created.ID
	key := "customer-key-" + testUnique(t)
	for _, bad := range []string{"", checkoutTestOperator, strings.Repeat("0", 64)} {
		f.requireResponse(t, http.MethodGet, path, bad, "", "", 401, "ORDER_UNAUTHORIZED")
		f.requireResponse(t, http.MethodPost, path+"/checkout", bad, key, "", 401, "ORDER_UNAUTHORIZED")
	}
	f.requireResponse(t, http.MethodGet, "/v1/customer/orders/absent", created.AccessToken, "", "", 401, "ORDER_UNAUTHORIZED")
	other := f.order(t)
	f.requireResponse(t, http.MethodGet, "/v1/customer/orders/"+other.ID, created.AccessToken, "", "", 401, "ORDER_UNAUTHORIZED")
	f.requireResponse(t, http.MethodPost, "/v1/customer/orders/"+other.ID+"/checkout", created.AccessToken, key, "", 401, "ORDER_UNAUTHORIZED")
	lookedUp := f.requireResponse(t, http.MethodGet, path, created.AccessToken, "", "", 200, "")
	if strings.Contains(string(lookedUp.body), "access_token") {
		t.Fatal("lookup revealed one-time capability")
	}
	f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+created.ID+"/pickup-token", created.AccessToken, "", "", 401, "OPERATOR_UNAUTHORIZED")
	f.requireResponse(t, http.MethodPost, path+"/checkout", created.AccessToken, key, `{"amount_cents":1}`, 400, "MALFORMED_REQUEST")
	g := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path+"/checkout", created.AccessToken, key, "", 201, ""))
	if g.OrderID != created.ID || g.AmountCents != created.Amount {
		t.Fatal("customer session not tied to authoritative reservation")
	}
	f.requireResponse(t, http.MethodPost, path+"/checkout", g.AccessToken, key, "", 401, "ORDER_UNAUTHORIZED")
	f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/"+g.ID, created.AccessToken, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/"+g.ID, g.AccessToken, "", "", 200, "")
	replay := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path+"/checkout", created.AccessToken, key, "", 200, ""))
	if replay.ID != g.ID {
		t.Fatal("customer replay duplicated resource")
	}
	operatorReplay := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+created.ID+"/checkout", checkoutTestOperator, "operator-key-"+testUnique(t), "", 200, ""))
	if operatorReplay.ID != g.ID {
		t.Fatal("customer/operator materialized different checkouts")
	}
	f.requireResponse(t, http.MethodPost, "/v1/orders", "", "", `{"offer_id":"demo-offer"}`, 412, "OFFER_UNAVAILABLE")
	var grants int
	var hash string
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*),min(token_hash) FROM order_access_grants`).Scan(&grants, &hash); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(created.AccessToken))
	if grants != 1 || hash != hex.EncodeToString(digest[:]) {
		t.Fatal("failed reservation left a grant or capability stored as plaintext")
	}
	// Reopen and migrate; no credential rotation/reissuance is implied.
	f.server.Close()
	f.store.Close()
	var err error
	f.store, err = store.Open(context.Background(), f.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.store.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer((API{Store: f.store}).Handler())
	f.requireResponse(t, http.MethodGet, path, created.AccessToken, "", "", 200, "")
	if _, err := f.pool.Exec(context.Background(), `UPDATE order_access_grants SET created_at=now()-interval '21 minutes',expires_at=now()-interval '1 minute' WHERE order_id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	f.requireResponse(t, http.MethodGet, path, created.AccessToken, "", "", 401, "ORDER_UNAUTHORIZED")
	f.requireResponse(t, http.MethodPost, path+"/checkout", created.AccessToken, key, "", 401, "ORDER_UNAUTHORIZED")
}

func TestCustomerCapabilityExpirationWhileCheckoutWaitsForLock(t *testing.T) {
	f := newCheckoutFixture(t)
	created, err := f.store.CreateOrderWithAccess(context.Background(), testUnique(t), "demo-offer")
	if err != nil {
		t.Fatal(err)
	}
	key := "waiting-customer-key-" + testUnique(t)
	digest := sha256.Sum256([]byte(key))
	tx, err := f.pool.Begin(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err := tx.Exec(context.Background(), `SELECT pg_advisory_xact_lock(hashtextextended($1,0))`, "local-checkout-key:"+hex.EncodeToString(digest[:])); err != nil {
		t.Fatal(err)
	}
	var lockerPID int
	if err := tx.QueryRow(context.Background(), `SELECT pg_backend_pid()`).Scan(&lockerPID); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, _, err := f.store.IssueCustomerLocalCheckout(ctx, created.ID, key, created.AccessToken)
		result <- err
	}()
	// Observe the actual blocking relation, not a sleep-based concurrency guess.
	deadline := time.Now().Add(3 * time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		if err := f.pool.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, lockerPID).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("checkout caller never waited on its key lock")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE order_access_grants SET created_at=now()-interval '21 minutes',expires_at=now()-interval '1 minute' WHERE order_id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-result; !errors.Is(err, store.ErrOrderAccessUnauthorized) {
		t.Fatalf("expired caller after lock: %v", err)
	}
	var charges, sessions int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM simulated_charges),(SELECT count(*) FROM local_checkout_sessions)`).Scan(&charges, &sessions); err != nil {
		t.Fatal(err)
	}
	if charges != 0 || sessions != 0 {
		t.Fatal("expired caller committed checkout writes")
	}
}

func TestCustomerOrderRoutesDisabledOutsideDemo(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	h := (API{}).Handler()
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		path := "/v1/customer/orders/any"
		if method == http.MethodPost {
			path += "/checkout"
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		if rec.Code != 404 {
			t.Fatalf("customer route outside demo=%d", rec.Code)
		}
	}
	for _, path := range []string{"/v1/orders/any", "/v1/operator/orders/any"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != 404 {
			t.Fatalf("legacy/operator lookup outside demo=%d", rec.Code)
		}
	}
}

func TestCustomerCapabilityAndStockRollbackTogether(t *testing.T) {
	f := newCheckoutFixture(t)
	ctx := context.Background()
	id := testUnique(t)
	if _, err := f.store.CreateOrderWithAccess(ctx, id, "demo-offer"); err != nil {
		t.Fatal(err)
	}
	offer := testUnique(t)
	if _, err := f.pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Rollback fixture',1400,1,1,120)`, offer); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.CreateOrderWithAccess(ctx, id, offer); err == nil {
		t.Fatal("duplicate order ID was accepted")
	}
	var stock, grants int
	if err := f.pool.QueryRow(ctx, `SELECT available_units,(SELECT count(*) FROM order_access_grants) FROM offers WHERE id=$1`, offer).Scan(&stock, &grants); err != nil {
		t.Fatal(err)
	}
	if stock != 1 || grants != 1 {
		t.Fatal("failed reservation did not roll back stock and capability together")
	}
}

func TestLegacyOrderLookupRequiresCapabilityAndOperatorLookupIsSeparate(t *testing.T) {
	f := newCheckoutFixture(t)
	created, err := f.store.CreateOrderWithAccess(context.Background(), testUnique(t), "demo-offer")
	if err != nil {
		t.Fatal(err)
	}
	legacyPath := "/v1/orders/" + created.ID
	for _, token := range []string{"", checkoutTestOperator, strings.Repeat("0", 64)} {
		f.requireResponse(t, http.MethodGet, legacyPath, token, "", "", 401, "ORDER_UNAUTHORIZED")
	}
	lookup := f.requireResponse(t, http.MethodGet, legacyPath, created.AccessToken, "", "", 200, "")
	if lookup.header.Get("Cache-Control") != "no-store" || strings.Contains(string(lookup.body), "access_token") {
		t.Fatal("legacy alias caches or reveals credential")
	}
	oldOrder := f.order(t)
	f.requireResponse(t, http.MethodGet, "/v1/orders/"+oldOrder.ID, created.AccessToken, "", "", 401, "ORDER_UNAUTHORIZED")
	operatorPath := "/v1/operator/orders/" + oldOrder.ID
	for _, token := range []string{"", created.AccessToken} {
		f.requireResponse(t, http.MethodGet, operatorPath, token, "", "", 401, "OPERATOR_UNAUTHORIZED")
	}
	operational := f.requireResponse(t, http.MethodGet, operatorPath, checkoutTestOperator, "", "", 200, "")
	if operational.header.Get("Cache-Control") != "no-store" || strings.Contains(string(operational.body), "access_token") {
		t.Fatal("operator lookup caches or issues customer access")
	}
	f.requireResponse(t, http.MethodGet, "/v1/operator/orders/absent", checkoutTestOperator, "", "", 404, "ORDER_NOT_FOUND")
	if _, err := f.pool.Exec(context.Background(), `UPDATE order_access_grants SET created_at=now()-interval '21 minutes',expires_at=now()-interval '1 minute' WHERE order_id=$1`, created.ID); err != nil {
		t.Fatal(err)
	}
	f.requireResponse(t, http.MethodGet, legacyPath, created.AccessToken, "", "", 401, "ORDER_UNAUTHORIZED")
}
