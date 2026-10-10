package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/lucaseufrasio/woovi-pix-examples/reservas/backend/internal/store"
)

func integrationAPI(t *testing.T) (*API, *store.Store) {
	t.Helper()
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL HTTP integration tests")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := pgxpool.NewWithConfig(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("reservations_http_%d", time.Now().UnixNano())
	if _, err := admin.Exec(context.Background(), `CREATE SCHEMA `+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	query := parsed.Query()
	query.Set("options", "-csearch_path="+schema+",public")
	parsed.RawQuery = query.Encode()
	st, err := store.Open(context.Background(), parsed.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(st.Close)
	if err := st.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	api := &API{Store: st, SimulatorToken: "simulator-token-12345678901234567890", OperatorToken: "operator-token-12345678901234567890"}
	return api, st
}

func TestReservationHTTPFlowEnforcesCapabilityAndSeparatePaymentState(t *testing.T) {
	api, _ := integrationAPI(t)
	startAt := nextBusinessDayAtNine(t)
	body, err := json.Marshal(map[string]string{"starts_at": startAt.Format(time.RFC3339)})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/reservations", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var created struct {
		Reservation struct {
			ID               string `json:"id"`
			ReservationState string `json:"reservation_state"`
			PaymentState     string `json:"payment_state"`
		} `json:"reservation"`
		Capability string `json:"capability"`
	}
	if err := json.NewDecoder(response.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Reservation.ID == "" || len(created.Capability) != 64 || created.Reservation.ReservationState != "held" || created.Reservation.PaymentState != "pending" {
		t.Fatalf("unexpected created reservation response: %+v", created)
	}

	privateRequest := httptest.NewRequest(http.MethodGet, "/v1/reservations/"+created.Reservation.ID, nil)
	privateResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(privateResponse, privateRequest)
	if privateResponse.Code != http.StatusUnauthorized {
		t.Fatalf("ID-only lookup should fail, got %d", privateResponse.Code)
	}

	paidRequest := httptest.NewRequest(http.MethodPost, "/v1/simulator/reservations/"+created.Reservation.ID+"/paid", nil)
	paidRequest.Header.Set("Authorization", "Bearer "+api.SimulatorToken)
	paidRequest.Header.Set("Idempotency-Key", "paid-event-key-000001")
	paidResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(paidResponse, paidRequest)
	if paidResponse.Code != http.StatusAccepted {
		t.Fatalf("simulator status=%d body=%s", paidResponse.Code, paidResponse.Body.String())
	}
	if _, err := api.Store.ProcessPaymentEvents(context.Background(), 20); err != nil {
		t.Fatal(err)
	}
	getRequest := httptest.NewRequest(http.MethodGet, "/v1/reservations/"+created.Reservation.ID, nil)
	getRequest.Header.Set("Authorization", "Bearer "+created.Capability)
	getResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(getResponse, getRequest)
	var current map[string]any
	if err := json.NewDecoder(getResponse.Body).Decode(&current); err != nil {
		t.Fatal(err)
	}
	if getResponse.Code != http.StatusOK || current["reservation_state"] != "confirmed" || current["payment_state"] != "paid" {
		t.Fatalf("payment and reservation state transition mismatch: %d %+v", getResponse.Code, current)
	}

	operatorRequest := httptest.NewRequest(http.MethodGet, "/v1/operator/reservations", nil)
	operatorRequest.Header.Set("Authorization", "Bearer "+api.OperatorToken)
	operatorResponse := httptest.NewRecorder()
	api.Handler().ServeHTTP(operatorResponse, operatorRequest)
	if operatorResponse.Code != http.StatusOK {
		t.Fatalf("operator list status=%d body=%s", operatorResponse.Code, operatorResponse.Body.String())
	}
}

func nextBusinessDayAtNine(t *testing.T) time.Time {
	t.Helper()
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, 1)
	for date.Weekday() == time.Saturday || date.Weekday() == time.Sunday {
		date = date.AddDate(0, 0, 1)
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 9, 0, 0, 0, location)
}
