package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

type API struct{ Store *store.Store }

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /offers", func(w http.ResponseWriter, r *http.Request) {
		offers, err := a.Store.ListOffers(r.Context())
		if err != nil {
			writeError(w, 500, "internal_error")
			return
		}
		writeJSON(w, 200, offers)
	})
	mux.HandleFunc("POST /orders", a.createOrder)
	mux.HandleFunc("GET /orders/{id}", a.getOrder)
	mux.HandleFunc("GET /operator/orders", a.listOrders)
	mux.HandleFunc("POST /operator/demo/reset", a.resetDemo)
	mux.HandleFunc("POST /dev/webhooks/paid", a.paidWebhook)
	return mux
}

func (a API) createOrder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OfferID string `json:"offer_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil || strings.TrimSpace(input.OfferID) == "" {
		writeError(w, 400, "invalid_request")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	order, err := a.Store.CreateOrder(r.Context(), id, input.OfferID)
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, 409, "offer_unavailable")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (a API) getOrder(w http.ResponseWriter, r *http.Request) {
	o, err := a.Store.GetOrder(r.Context(), r.PathValue("id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, 404, "not_found")
		return
	}
	if err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	writeJSON(w, 200, o)
}
func (a API) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := a.Store.ListOrders(r.Context())
	if err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	writeJSON(w, 200, orders)
}
func (a API) resetDemo(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, 404, "not_found")
		return
	}
	if err := a.Store.ResetDemo(r.Context()); err != nil {
		writeError(w, 500, "internal_error")
		return
	}
	writeJSON(w, 200, map[string]string{"status": "reset"})
}

func (a API) paidWebhook(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	var in struct {
		OrderID  string `json:"order_id"`
		EventKey string `json:"event_key"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if dec.Decode(&in) != nil || in.OrderID == "" || in.EventKey == "" {
		writeError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if _, err := a.Store.GetOrder(r.Context(), in.OrderID); err != nil {
		writeError(w, http.StatusNotFound, "not_found")
		return
	}
	sum := sha256.Sum256([]byte(in.EventKey))
	eventID := hex.EncodeToString(sum[:])
	_, err := a.Store.PersistPaidEvent(r.Context(), eventID, in.OrderID, in.EventKey)
	if errors.Is(err, store.ErrEventKeyConflict) {
		writeError(w, http.StatusConflict, "idempotency_conflict")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}
func newID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, map[string]string{"error": code})
}
