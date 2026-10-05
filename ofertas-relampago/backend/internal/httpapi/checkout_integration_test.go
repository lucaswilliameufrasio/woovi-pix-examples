package httpapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

const checkoutTestOperator = "isolated-local-checkout-operator-0123456789"

type checkoutFixture struct {
	store  *store.Store
	pool   *pgxpool.Pool
	server *httptest.Server
	dbURL  string
}

func newCheckoutFixture(t *testing.T) *checkoutFixture {
	t.Helper()
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("DEMO_OPERATOR_TOKEN", checkoutTestOperator)
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	root, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	schema := "checkout_test_" + testUnique(t)
	if _, err := root.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		root.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := root.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Errorf("cleanup isolated checkout schema: %v", err)
		}
		root.Close()
	})
	u, err := url.Parse(dbURL)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	f := &checkoutFixture{dbURL: u.String()}
	f.pool, err = pgxpool.New(ctx, f.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(f.pool.Close)
	f.store, err = store.Open(ctx, f.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.store.Close() })
	if err := f.store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	f.server = httptest.NewServer((API{Store: f.store}).Handler())
	t.Cleanup(func() { f.server.Close() })
	return f
}

func (f *checkoutFixture) order(t *testing.T) store.Order {
	t.Helper()
	ctx := context.Background()
	id := testUnique(t)
	if _, err := f.pool.Exec(ctx, `INSERT INTO offers(id,title,price_cents,total_units,available_units,reservation_ttl_seconds) VALUES($1,'Isolated checkout',1350,1,1,120)`, id); err != nil {
		t.Fatal(err)
	}
	o, err := f.store.CreateOrder(ctx, testUnique(t), id)
	if err != nil {
		t.Fatal(err)
	}
	return o
}

type checkoutHTTPResult struct {
	status int
	header http.Header
	body   []byte
}

func (f *checkoutFixture) request(method, path, token, key, body string) (checkoutHTTPResult, error) {
	req, err := http.NewRequest(method, f.server.URL+path, strings.NewReader(body))
	if err != nil {
		return checkoutHTTPResult{}, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return checkoutHTTPResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := io.ReadAll(resp.Body)
	return checkoutHTTPResult{status: resp.StatusCode, header: resp.Header, body: payload}, err
}

func (f *checkoutFixture) requireResponse(t *testing.T, method, path, token, key, body string, status int, code string) checkoutHTTPResult {
	t.Helper()
	r, err := f.request(method, path, token, key, body)
	if err != nil {
		t.Fatal(err)
	}
	if r.status != status {
		t.Fatalf("%s %s status=%d, want %d (body omitted)", method, path, r.status, status)
	}
	if code != "" {
		var e apiError
		if err := json.Unmarshal(r.body, &e); err != nil {
			t.Fatal(err)
		}
		if e.ErrorCode != code || e.Message == "" {
			t.Fatalf("error contract=%s, want %s", e.ErrorCode, code)
		}
	}
	return r
}

func decodeCheckoutGrant(t *testing.T, r checkoutHTTPResult) store.LocalCheckoutGrant {
	t.Helper()
	var g store.LocalCheckoutGrant
	if err := json.Unmarshal(r.body, &g); err != nil {
		t.Fatal(err)
	}
	if len(g.AccessToken) != 64 || g.ID == "" {
		t.Fatal("missing checkout ID/credential (values omitted)")
	}
	return g
}

func TestLocalCheckoutAuthorizationAndAuthoritativeBinding(t *testing.T) {
	f := newCheckoutFixture(t)
	o := f.order(t)
	path := "/v1/operator/orders/" + o.ID + "/checkout"
	key := "local-checkout-key-" + testUnique(t)
	f.requireResponse(t, http.MethodPost, path, "", key, "", 401, "OPERATOR_UNAUTHORIZED")
	f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, "", "", 422, "INVALID_PARAMS")
	for _, invalid := range []string{"short", strings.Repeat("a", 129), "key with spaces 123456", "non-ascii-é-123456"} {
		f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, invalid, "", 422, "INVALID_PARAMS")
	}
	f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, `{"amount_cents":1}`, 400, "MALFORMED_REQUEST")
	f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, strings.Repeat("x", 5000), 413, "PAYLOAD_TOO_LARGE")
	var before int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM simulated_charges`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 0 {
		t.Fatal("rejected commands created a charge")
	}
	r := f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 201, "")
	g := decodeCheckoutGrant(t, r)
	if g.OrderID != o.ID || g.AmountCents != 1350 || g.ChargeID != "sim-"+o.ID || g.Status != "pending" || g.ChargeStatus != "pending" || !g.ExpiresAt.Equal(o.ExpiresAt) {
		t.Fatal("checkout does not bind to authoritative order/charge")
	}
	if !g.TokenExpiresAt.After(time.Now()) || g.TokenExpiresAt.After(time.Now().Add(10*time.Minute+time.Second)) {
		t.Fatal("credential TTL is not bounded")
	}
	if r.header.Get("Cache-Control") != "no-store" || r.header.Get("Referrer-Policy") != "no-referrer" {
		t.Fatal("credential response must not be cached")
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(r.body, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["pix_copy_paste"]; ok {
		t.Fatal("local checkout must not invent a Pix payload")
	}
	if string(payload["mode"]) != `"local_simulation"` || string(payload["currency"]) != `"BRL"` {
		t.Fatal("missing local mode/currency")
	}
	lookup := "/v1/checkout-sessions/" + g.ID
	for _, token := range []string{"", checkoutTestOperator, strings.Repeat("0", 64)} {
		f.requireResponse(t, http.MethodGet, lookup, token, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	}
	f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/absent", g.AccessToken, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	got := f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 200, "")
	if strings.Contains(string(got.body), "access_token") || got.header.Get("Cache-Control") != "no-store" {
		t.Fatal("read leaked credential or cache metadata")
	}
	f.requireResponse(t, http.MethodPost, path, g.AccessToken, key, "", 401, "OPERATOR_UNAUTHORIZED")
	f.requireResponse(t, http.MethodPost, lookup, g.AccessToken, "", "", 405, "")
	replay := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 200, ""))
	if replay.ID != g.ID || replay.ChargeID != g.ChargeID || replay.AccessToken == g.AccessToken {
		t.Fatal("replay did not recover resource with a fresh credential")
	}
	f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 200, "")
	f.requireResponse(t, http.MethodGet, lookup, replay.AccessToken, "", "", 200, "")
	other := f.order(t)
	f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+other.ID+"/checkout", checkoutTestOperator, key, "", 409, "IDEMPOTENCY_CONFLICT")
	otherGrant := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+other.ID+"/checkout", checkoutTestOperator, "other-key-"+testUnique(t), "", 201, ""))
	f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/"+otherGrant.ID, g.AccessToken, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	var tokenHash, keyHash string
	if err := f.pool.QueryRow(context.Background(), `SELECT token_hash FROM local_checkout_grants WHERE checkout_id=$1 ORDER BY created_at LIMIT 1`, g.ID).Scan(&tokenHash); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `SELECT key_hash FROM local_checkout_keys WHERE checkout_id=$1`, g.ID).Scan(&keyHash); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(g.AccessToken))
	if tokenHash != hex.EncodeToString(digest[:]) || keyHash == key {
		t.Fatal("plaintext credential/key persisted")
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE local_checkout_grants SET created_at=now()-interval '11 minutes',expires_at=now()-interval '1 minute' WHERE token_hash=$1`, tokenHash); err != nil {
		t.Fatal(err)
	}
	f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	f.requireResponse(t, http.MethodGet, lookup, replay.AccessToken, "", "", 200, "")
	// Closing and reopening the store models a process restart; re-migration
	// must preserve session IDs, keys and grants.
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
	f.requireResponse(t, http.MethodGet, lookup, replay.AccessToken, "", "", 200, "")
	if g2 := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 200, "")); g2.ID != g.ID {
		t.Fatal("restart lost idempotency")
	}
}

func TestLocalCheckoutConcurrentIssuanceAndKeyConflicts(t *testing.T) {
	f := newCheckoutFixture(t)
	// Independent pools/servers share only PostgreSQL, as separate backend
	// instances would. No process-local mutex may enforce idempotency.
	secondStore, err := store.Open(context.Background(), f.dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(secondStore.Close)
	secondServer := httptest.NewServer((API{Store: secondStore}).Handler())
	t.Cleanup(secondServer.Close)
	second := *f
	second.server = secondServer
	o := f.order(t)
	path := "/v1/operator/orders/" + o.ID + "/checkout"
	for _, sameKey := range []bool{true, false} {
		const calls = 12
		results := make(chan checkoutHTTPResult, calls)
		errs := make(chan error, calls)
		var wg sync.WaitGroup
		for i := range calls {
			wg.Go(func() {
				key := "concurrent-same-key-123456"
				if !sameKey {
					key = fmt.Sprintf("concurrent-key-%016d", i)
				}
				target := f
				if i%2 == 1 {
					target = &second
				}
				r, err := target.request(http.MethodPost, path, checkoutTestOperator, key, "")
				results <- r
				errs <- err
			})
		}
		wg.Wait()
		close(results)
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		id := ""
		created := 0
		for r := range results {
			if r.status != 200 && r.status != 201 {
				t.Fatalf("concurrent status=%d", r.status)
			}
			if r.status == 201 {
				created++
			}
			g := decodeCheckoutGrant(t, r)
			if id != "" && id != g.ID {
				t.Fatal("concurrent requests created different sessions")
			}
			id = g.ID
			f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/"+id, g.AccessToken, "", "", 200, "")
		}
		if sameKey && created != 1 || !sameKey && created != 0 {
			t.Fatalf("unexpected new sessions=%d", created)
		}
	}
	var sessions, charges int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM local_checkout_sessions),(SELECT count(*) FROM simulated_charges)`).Scan(&sessions, &charges); err != nil {
		t.Fatal(err)
	}
	if sessions != 1 || charges != 1 {
		t.Fatal("issuance duplicated a session or charge")
	}
	a, b := f.order(t), f.order(t)
	results := make(chan checkoutHTTPResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, o := range []store.Order{a, b} {
		wg.Go(func() {
			r, err := f.request(http.MethodPost, "/v1/operator/orders/"+o.ID+"/checkout", checkoutTestOperator, "cross-order-key-123456", "")
			results <- r
			errs <- err
		})
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	counts := map[int]int{}
	for r := range results {
		counts[r.status]++
	}
	if counts[201] != 1 || counts[409] != 1 {
		t.Fatalf("cross-order race statuses=%v", counts)
	}
}

func TestLocalCheckoutTracksWorkerExpiryAndPaymentException(t *testing.T) {
	f := newCheckoutFixture(t)
	for _, late := range []bool{false, true} {
		o := f.order(t)
		path := "/v1/operator/orders/" + o.ID + "/checkout"
		key := "worker-checkout-" + testUnique(t)
		g := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 201, ""))
		lookup := "/v1/checkout-sessions/" + g.ID
		if late {
			if _, err := f.pool.Exec(context.Background(), `UPDATE orders SET expires_at=now()-interval '1 second' WHERE id=$1`, o.ID); err != nil {
				t.Fatal(err)
			}
			r := f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 200, "")
			var expired store.LocalCheckout
			if err := json.Unmarshal(r.body, &expired); err != nil {
				t.Fatal(err)
			}
			if expired.Status != "expired" || expired.OrderState != "pending_payment" {
				t.Fatal("deadline must suppress pending before expirer runs")
			}
			if _, err := f.store.ExpireOrders(context.Background()); err != nil {
				t.Fatal(err)
			}
			f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 200, "")
		}
		if _, err := f.store.MarkSimulatedChargePaid(context.Background(), o.ID); err != nil {
			t.Fatal(err)
		}
		eventKey := "local-paid-" + testUnique(t)
		if _, err := f.store.PersistPaidEvent(context.Background(), testUnique(t), o.ID, eventKey); err != nil {
			t.Fatal(err)
		}
		if !late {
			r := f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 200, "")
			var pending store.LocalCheckout
			if err := json.Unmarshal(r.body, &pending); err != nil {
				t.Fatal(err)
			}
			if pending.Status != "pending" || pending.ChargeStatus != "paid" {
				t.Fatal("financial event must not bypass worker")
			}
		}
		if _, err := f.store.ProcessPaymentEvents(context.Background(), 32); err != nil {
			t.Fatal(err)
		}
		if late {
			f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 412, "PAYMENT_EXCEPTION")
			f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 412, "PAYMENT_EXCEPTION")
		} else {
			r := f.requireResponse(t, http.MethodGet, lookup, g.AccessToken, "", "", 200, "")
			var paid store.LocalCheckout
			if err := json.Unmarshal(r.body, &paid); err != nil {
				t.Fatal(err)
			}
			if paid.Status != "paid" || paid.OrderState != "paid" || paid.ChargeStatus != "paid" {
				t.Fatal("worker state not reflected")
			}
			g2 := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, key, "", 200, ""))
			if g2.ID != g.ID || g2.Status != "paid" {
				t.Fatal("terminal replay created another checkout")
			}
		}
	}
	// No previous checkout: expired, already-paid or mismatched charge cannot
	// materialize a fresh session. All inserts/keys/grants must roll back.
	for _, state := range []string{"expired", "paid", "pending_payment"} {
		o := f.order(t)
		if _, err := f.pool.Exec(context.Background(), `UPDATE orders SET state=$2 WHERE id=$1`, o.ID, state); err != nil {
			t.Fatal(err)
		}
		if state == "pending_payment" {
			if _, err := f.store.CreateSimulatedCharge(context.Background(), o.ID, 1); err != nil {
				t.Fatal(err)
			}
		}
		status, code := 412, "CHECKOUT_NOT_ELIGIBLE"
		if state == "pending_payment" {
			status, code = 409, "IDEMPOTENCY_CONFLICT"
		}
		f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+o.ID+"/checkout", checkoutTestOperator, "ineligible-key-"+testUnique(t), "", status, code)
		var sessions int
		if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM local_checkout_sessions WHERE order_id=$1`, o.ID).Scan(&sessions); err != nil {
			t.Fatal(err)
		}
		if sessions != 0 {
			t.Fatal("ineligible command persisted a session")
		}
	}
}

func TestLocalCheckoutDisabledOutsideDemo(t *testing.T) {
	t.Setenv("DEMO_MODE", "false")
	handler := (API{}).Handler()
	for _, tc := range []struct{ method, path string }{{http.MethodGet, "/v1/checkout-sessions/anything"}, {http.MethodPost, "/v1/operator/orders/anything/checkout"}} {
		r := httptest.NewRecorder()
		handler.ServeHTTP(r, httptest.NewRequest(tc.method, tc.path, nil))
		if r.Code != 404 {
			t.Fatalf("non-demo checkout status=%d", r.Code)
		}
	}
}

func TestLocalCheckoutRecoversExistingChargeAndRevokesOnIsolatedReset(t *testing.T) {
	f := newCheckoutFixture(t)
	o := f.order(t)
	charge, err := f.store.CreateSimulatedCharge(context.Background(), o.ID, o.Amount)
	if err != nil {
		t.Fatal(err)
	}
	path := "/v1/operator/orders/" + o.ID + "/checkout"
	g := decodeCheckoutGrant(t, f.requireResponse(t, http.MethodPost, path, checkoutTestOperator, "recovered-key-"+testUnique(t), "", 201, ""))
	if g.ChargeID != charge.ID {
		t.Fatal("session did not recover the existing charge")
	}
	// A financial paid charge without a session must wait for reconciliation,
	// even while its order is still pending_payment.
	pending := f.order(t)
	if _, err := f.store.CreateSimulatedCharge(context.Background(), pending.ID, pending.Amount); err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.MarkSimulatedChargePaid(context.Background(), pending.ID); err != nil {
		t.Fatal(err)
	}
	f.requireResponse(t, http.MethodPost, "/v1/operator/orders/"+pending.ID+"/checkout", checkoutTestOperator, "paid-charge-key-"+testUnique(t), "", 412, "CHECKOUT_NOT_ELIGIBLE")
	f.requireResponse(t, http.MethodPost, "/v1/operator/orders/missing/checkout", checkoutTestOperator, "missing-order-key-123456", "", 404, "ORDER_NOT_FOUND")
	var keys, grants int
	if err := f.pool.QueryRow(context.Background(), `SELECT (SELECT count(*) FROM local_checkout_keys),(SELECT count(*) FROM local_checkout_grants)`).Scan(&keys, &grants); err != nil {
		t.Fatal(err)
	}
	if keys != 1 || grants != 1 {
		t.Fatal("rejected issuance committed keys/grants")
	}
	// This reset is solely in the randomly named schema created by this test;
	// never in the operator's demo data. Cascades must revoke old grants/keys.
	if err := f.store.ResetDemo(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.requireResponse(t, http.MethodGet, "/v1/checkout-sessions/"+g.ID, g.AccessToken, "", "", 401, "CHECKOUT_UNAUTHORIZED")
	var sessions int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM local_checkout_sessions`).Scan(&sessions); err != nil {
		t.Fatal(err)
	}
	if sessions != 0 {
		t.Fatal("isolated reset did not revoke session")
	}
}

func TestLocalCheckoutRejectsDuplicateIdempotencyHeadersBeforeStore(t *testing.T) {
	t.Setenv("DEMO_MODE", "true")
	t.Setenv("DEMO_OPERATOR_TOKEN", checkoutTestOperator)
	req := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/any/checkout", nil)
	req.Header.Set("Authorization", "Bearer "+checkoutTestOperator)
	req.Header.Add("Idempotency-Key", strings.Repeat("a", 16))
	req.Header.Add("Idempotency-Key", strings.Repeat("b", 16))
	rec := httptest.NewRecorder()
	(API{}).Handler().ServeHTTP(rec, req)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `"error_code":"INVALID_PARAMS"`) {
		t.Fatal("ambiguous idempotency headers reached the store")
	}
}
