package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/click-collect/backend/internal/store"
)

const operatorToken = "operator-token-for-local-integration-tests-0001"
const simulatorToken = "simulator-token-for-local-integration-tests-001"

func apiIntegrationServer(t *testing.T) (http.Handler, *store.Store) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.Ping(ctx); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	bytes := make([]byte, 8)
	if _, err := rand.Read(bytes); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "click_collect_http_" + hex.EncodeToString(bytes)
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`); admin.Close() })
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema)
	parsed.RawQuery = query.Encode()
	st, err := store.Open(ctx, parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return New(st, operatorToken, simulatorToken), st
}

func perform(handler http.Handler, method, target, token, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestHTTPFlowKeepsAccessPrivateAndCompletesLocalPickupOnce(t *testing.T) {
	handler, st := apiIntegrationServer(t)
	products := perform(handler, http.MethodGet, "/v1/products", "", "")
	if products.Code != http.StatusOK || !strings.Contains(products.Body.String(), `"price_cents":890`) {
		t.Fatalf("products: %d %s", products.Code, products.Body.String())
	}
	created := perform(handler, http.MethodPost, "/v1/orders", "", `{"product_id":"house-cake"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var reserved struct {
		ID     string `json:"id"`
		Token  string `json:"order_access_token"`
		Pickup string `json:"pickup_code"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &reserved); err != nil {
		t.Fatal(err)
	}
	if reserved.ID == "" || reserved.Token == "" || reserved.Pickup == "" {
		t.Fatalf("reservation credential fields missing: %+v", reserved)
	}
	unauthorized := perform(handler, http.MethodGet, "/v1/orders/"+reserved.ID, "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized order access status=%d", unauthorized.Code)
	}
	private := perform(handler, http.MethodGet, "/v1/orders/"+reserved.ID, reserved.Token, "")
	if private.Code != http.StatusOK || !strings.Contains(private.Body.String(), `"payment_state":"pending"`) {
		t.Fatalf("private order: %d %s", private.Code, private.Body.String())
	}
	charge := performWithDemoToken(handler, http.MethodPost, "/v1/simulator/orders/"+reserved.ID+"/charge", simulatorToken)
	if charge.Code != http.StatusCreated {
		t.Fatalf("charge: %d %s", charge.Code, charge.Body.String())
	}
	var firstCharge struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(charge.Body.Bytes(), &firstCharge); err != nil {
		t.Fatal(err)
	}
	repeatedCharge := performWithDemoToken(handler, http.MethodPost, "/v1/simulator/orders/"+reserved.ID+"/charge", simulatorToken)
	var secondCharge struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(repeatedCharge.Body.Bytes(), &secondCharge); err != nil {
		t.Fatal(err)
	}
	if repeatedCharge.Code != http.StatusCreated || secondCharge.ID != firstCharge.ID {
		t.Fatalf("repeated charge: %d %s", repeatedCharge.Code, repeatedCharge.Body.String())
	}
	confirmed := performWithDemoToken(handler, http.MethodPost, "/v1/simulator/orders/"+reserved.ID+"/confirm", simulatorToken)
	if confirmed.Code != http.StatusAccepted {
		t.Fatalf("confirm: %d %s", confirmed.Code, confirmed.Body.String())
	}
	confirmedAgain := performWithDemoToken(handler, http.MethodPost, "/v1/simulator/orders/"+reserved.ID+"/confirm", simulatorToken)
	if confirmedAgain.Code != http.StatusAccepted {
		t.Fatalf("repeated confirm: %d %s", confirmedAgain.Code, confirmedAgain.Body.String())
	}
	if count, err := st.ProcessPaymentEvents(context.Background(), 10); err != nil || count != 1 {
		t.Fatalf("process payment events: count=%d err=%v", count, err)
	}
	var orderState string
	current := perform(handler, http.MethodGet, "/v1/orders/"+reserved.ID, reserved.Token, "")
	var response struct {
		PaymentState string `json:"payment_state"`
	}
	if err := json.Unmarshal(current.Body.Bytes(), &response); err == nil {
		orderState = response.PaymentState
	}
	if orderState != "paid" {
		t.Fatalf("payment did not reach backend: %s", orderState)
	}
	queueRequest := httptest.NewRequest(http.MethodGet, "/v1/operator/orders", nil)
	queueRequest.Header.Set("X-Demo-Token", operatorToken)
	queueRecorder := httptest.NewRecorder()
	handler.ServeHTTP(queueRecorder, queueRequest)
	if queueRecorder.Code != http.StatusOK || !strings.Contains(queueRecorder.Body.String(), reserved.ID) {
		t.Fatalf("operator queue: %d %s", queueRecorder.Code, queueRecorder.Body.String())
	}
	readyRequest := httptest.NewRequest(http.MethodPost, "/v1/operator/orders/"+reserved.ID+"/ready", nil)
	readyRequest.Header.Set("X-Demo-Token", operatorToken)
	readyRecorder := httptest.NewRecorder()
	handler.ServeHTTP(readyRecorder, readyRequest)
	if readyRecorder.Code != http.StatusOK {
		t.Fatalf("mark ready: %d %s", readyRecorder.Code, readyRecorder.Body.String())
	}
	pickupBody, err := json.Marshal(map[string]string{"pickup_code": reserved.Pickup})
	if err != nil {
		t.Fatal(err)
	}
	firstPickup := performOperatorPickup(handler, reserved.ID, string(pickupBody))
	if firstPickup.Code != http.StatusOK {
		t.Fatalf("pickup: %d %s", firstPickup.Code, firstPickup.Body.String())
	}
	secondPickup := performOperatorPickup(handler, reserved.ID, string(pickupBody))
	if secondPickup.Code != http.StatusConflict {
		t.Fatalf("duplicate pickup: %d %s", secondPickup.Code, secondPickup.Body.String())
	}
}

func TestCustomerAuthorizedLocalSimulationIsDurableAndIdempotent(t *testing.T) {
	handler, st := apiIntegrationServer(t)
	created := perform(handler, http.MethodPost, "/v1/orders", "", `{"product_id":"house-cake"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	var reserved struct {
		ID    string `json:"id"`
		Token string `json:"order_access_token"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &reserved); err != nil {
		t.Fatal(err)
	}
	unauthorized := perform(handler, http.MethodPost, "/v1/orders/"+reserved.ID+"/demo-payment", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized demo payment status=%d", unauthorized.Code)
	}
	for range 2 {
		response := perform(handler, http.MethodPost, "/v1/orders/"+reserved.ID+"/demo-payment", reserved.Token, "")
		if response.Code != http.StatusAccepted {
			t.Fatalf("demo payment: %d %s", response.Code, response.Body.String())
		}
	}
	count, err := st.ProcessPaymentEvents(context.Background(), 10)
	if err != nil || count != 1 {
		t.Fatalf("processed=%d err=%v", count, err)
	}
	order := perform(handler, http.MethodGet, "/v1/orders/"+reserved.ID, reserved.Token, "")
	if order.Code != http.StatusOK || !strings.Contains(order.Body.String(), `"payment_state":"paid"`) {
		t.Fatalf("order: %d %s", order.Code, order.Body.String())
	}
}

func performWithDemoToken(handler http.Handler, method, target, token string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, target, nil)
	request.Header.Set("X-Demo-Token", token)
	handler.ServeHTTP(recorder, request)
	return recorder
}

func performOperatorPickup(handler http.Handler, orderID, body string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/v1/operator/orders/%s/pickup", orderID), strings.NewReader(body))
	request.Header.Set("X-Demo-Token", operatorToken)
	handler.ServeHTTP(recorder, request)
	return recorder
}
