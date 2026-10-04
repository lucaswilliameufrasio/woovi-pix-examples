package simulator

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

type Charge struct {
	ID          string `json:"id"`
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Status      string `json:"status"`
}
type Server struct {
	store   *store.Store
	lookup  func(string) (int64, error)
	deliver func(string, string) error
}

func New(st *store.Store, lookup func(string) (int64, error), deliver func(string, string) error) *Server {
	return &Server{store: st, lookup: lookup, deliver: deliver}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /charges", s.create)
	mux.HandleFunc("POST /charges/{orderID}/pay", s.pay)
	mux.HandleFunc("GET /charges/{orderID}", s.get)
	mux.HandleFunc("POST /dev/webhooks/{orderID}/paid", s.pay)
	mux.HandleFunc("POST /dev/webhooks/{orderID}/late-paid", s.latePay)
	mux.HandleFunc("POST /dev/scenarios/{name}", s.scenario)
	return mux
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	var in struct {
		OrderID string `json:"order_id"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.OrderID == "" {
		http.Error(w, "invalid request", 400)
		return
	}
	amount, err := s.lookup(in.OrderID)
	if err != nil {
		http.Error(w, "order not found", 404)
		return
	}
	saved, err := s.store.CreateSimulatedCharge(r.Context(), in.OrderID, amount)
	if errors.Is(err, store.ErrChargeConflict) {
		http.Error(w, "idempotency conflict", http.StatusConflict)
		return
	}
	if err != nil {
		http.Error(w, "charge persistence failed", http.StatusInternalServerError)
		return
	}
	c := Charge{ID: saved.ID, OrderID: saved.OrderID, AmountCents: saved.AmountCents, Status: saved.Status}
	write(w, c)
}
func (s *Server) pay(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("orderID")
	charge, err := s.store.MarkSimulatedChargePaid(r.Context(), orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "charge persistence failed", http.StatusInternalServerError)
		return
	}
	if s.deliver == nil || s.deliver(charge.OrderID, "paid:"+charge.OrderID) != nil {
		http.Error(w, "delivery failed", http.StatusBadGateway)
		return
	}
	write(w, map[string]any{"status": "webhook accepted", "at": time.Now().UTC()})
}
func (s *Server) latePay(w http.ResponseWriter, r *http.Request) {
	if s.deliver == nil || s.deliver(r.PathValue("orderID"), "late-paid:"+r.PathValue("orderID")) != nil {
		http.Error(w, "delivery failed", http.StatusBadGateway)
		return
	}
	write(w, map[string]any{"status": "late webhook accepted", "at": time.Now().UTC()})
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	saved, err := s.store.GetSimulatedCharge(r.Context(), r.PathValue("orderID"))
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "not found", 404)
		return
	}
	if err != nil {
		http.Error(w, "charge lookup failed", http.StatusInternalServerError)
		return
	}
	c := Charge{ID: saved.ID, OrderID: saved.OrderID, AmountCents: saved.AmountCents, Status: saved.Status}
	write(w, c)
}
func (s *Server) scenario(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	switch r.PathValue("name") {
	case "duplicate-paid":
		s.pay(w, r)
	case "timeout":
		http.Error(w, "simulated timeout after charge creation", http.StatusGatewayTimeout)
	case "rate-limit":
		w.Header().Set("Retry-After", "1")
		http.Error(w, "simulated rate limit", http.StatusTooManyRequests)
	case "delay":
		time.Sleep(250 * time.Millisecond)
		write(w, map[string]string{"status": "delayed"})
	default:
		http.Error(w, "unknown scenario", http.StatusNotFound)
	}
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
