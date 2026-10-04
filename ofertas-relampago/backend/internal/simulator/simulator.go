package simulator

import (
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

type Charge struct {
	ID          string `json:"id"`
	OrderID     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Status      string `json:"status"`
	OrderState  string `json:"order_state"`
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
	mux.HandleFunc("POST /v1/charges", s.create)
	mux.HandleFunc("GET /v1/charges/{order_id}", s.get)
	if os.Getenv("DEMO_MODE") == "true" {
		mux.HandleFunc("POST /v1/charges/{order_id}/retry", s.createForOrder)
		mux.HandleFunc("POST /v1/charges/{order_id}/pay", s.pay)
		mux.HandleFunc("POST /v1/dev/webhooks/{order_id}/paid", s.pay)
		mux.HandleFunc("POST /v1/dev/webhooks/{order_id}/late_paid", s.latePay)
		mux.HandleFunc("POST /v1/dev/scenarios/{scenario_name}", s.scenario)
		mux.HandleFunc("POST /v1/dev/scenarios/{scenario_name}/{order_id}", s.scenarioForOrder)
	}
	return mux
}
func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var in struct {
		OrderID string `json:"order_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if in.OrderID == "" {
		writeValidationError(w, "order_id", "deve ser informado")
		return
	}
	if strings.TrimSpace(in.OrderID) != in.OrderID {
		writeValidationError(w, "order_id", "não deve conter espaços no início ou no fim")
		return
	}
	s.createCharge(w, r, in.OrderID)
}

func (s *Server) createForOrder(w http.ResponseWriter, r *http.Request) {
	s.createCharge(w, r, r.PathValue("order_id"))
}

func (s *Server) createCharge(w http.ResponseWriter, r *http.Request, orderID string) {
	amount, err := s.lookup(orderID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		} else {
			writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		}
		return
	}
	saved, err := s.store.CreateSimulatedCharge(r.Context(), orderID, amount)
	if errors.Is(err, store.ErrChargeConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	c := Charge{ID: saved.ID, OrderID: saved.OrderID, AmountCents: saved.AmountCents, Status: saved.Status, OrderState: saved.OrderState}
	write(w, c)
}
func (s *Server) pay(w http.ResponseWriter, r *http.Request) {
	orderID := r.PathValue("order_id")
	charge, err := s.store.MarkSimulatedChargePaid(r.Context(), orderID)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHARGE_NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	order, err := s.store.GetOrder(r.Context(), charge.OrderID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	if s.deliver == nil || s.deliver(charge.OrderID, "paid:"+charge.OrderID) != nil {
		writeError(w, http.StatusBadGateway, "DEPENDENCY_REQUEST")
		return
	}
	status := "webhook accepted"
	if order.State == "expired" {
		status = "late payment recorded for review"
	}
	write(w, map[string]any{"status": status, "at": time.Now().UTC()})
}
func (s *Server) latePay(w http.ResponseWriter, r *http.Request) {
	if s.deliver == nil || s.deliver(r.PathValue("order_id"), "late-paid:"+r.PathValue("order_id")) != nil {
		writeError(w, http.StatusBadGateway, "DEPENDENCY_REQUEST")
		return
	}
	write(w, map[string]any{"status": "late webhook accepted", "at": time.Now().UTC()})
}
func (s *Server) get(w http.ResponseWriter, r *http.Request) {
	saved, err := s.store.GetSimulatedCharge(r.Context(), r.PathValue("order_id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "CHARGE_NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	c := Charge{ID: saved.ID, OrderID: saved.OrderID, AmountCents: saved.AmountCents, Status: saved.Status, OrderState: saved.OrderState}
	write(w, c)
}
func (s *Server) scenario(w http.ResponseWriter, r *http.Request) {
	switch r.PathValue("scenario_name") {
	case "timeout":
		orderID := r.URL.Query().Get("order_id")
		if orderID == "" {
			writeValidationError(w, "order_id", "deve ser informado")
			return
		}
		amount, err := s.lookup(orderID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
			} else {
				writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
			}
			return
		}
		if _, err := s.store.CreateSimulatedCharge(r.Context(), orderID, amount); err != nil {
			writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
			return
		}
		writeError(w, http.StatusGatewayTimeout, "SIMULATED_TIMEOUT")
	case "rate-limit":
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, "SIMULATED_RATE_LIMIT")
	case "delay":
		time.Sleep(250 * time.Millisecond)
		write(w, map[string]string{"status": "delayed"})
	default:
		writeError(w, http.StatusNotFound, "SCENARIO_NOT_FOUND")
	}
}

func (s *Server) scenarioForOrder(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("scenario_name") != "duplicate_paid" {
		writeError(w, http.StatusNotFound, "SCENARIO_NOT_FOUND")
		return
	}
	s.pay(w, r)
}
func write(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code string) {
	messages := map[string]string{
		"MALFORMED_REQUEST":          "A requisição está malformada.",
		"INVALID_PARAMS":             "Parâmetros inválidos.",
		"ORDER_NOT_FOUND":            "Pedido não encontrado.",
		"CHARGE_NOT_FOUND":           "Cobrança não encontrada.",
		"IDEMPOTENCY_CONFLICT":       "A chave de idempotência conflita com uma cobrança existente.",
		"PAYMENT_SIMULATOR_DELIVERY": "Não foi possível entregar o evento ao backend local.",
		"DEPENDENCY_REQUEST":         "Não foi possível concluir a solicitação a uma dependência.",
		"SIMULATED_TIMEOUT":          "Timeout simulado após persistir a cobrança; consulte ou repita usando a mesma referência.",
		"SIMULATED_RATE_LIMIT":       "Limite simulado; tente novamente após o intervalo informado.",
		"SCENARIO_NOT_FOUND":         "Cenário não encontrado.",
		"PAYLOAD_TOO_LARGE":          "O corpo da requisição excede o limite permitido.",
		"UNEXPECTED_ERROR":           "Ocorreu um erro inesperado.",
	}
	message, ok := messages[code]
	if !ok || status >= http.StatusInternalServerError && status != http.StatusGatewayTimeout && status != http.StatusBadGateway {
		code, message = "UNEXPECTED_ERROR", "Ocorreu um erro inesperado."
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := map[string]any{"message": message, "error_code": code}
	if status == http.StatusTooManyRequests {
		body["extra"] = map[string]int{"retry_after_seconds": 1}
	}
	_ = json.NewEncoder(w).Encode(body)
}

func writeValidationError(w http.ResponseWriter, field, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(map[string]any{"message": "Parâmetros inválidos.", "error_code": "INVALID_PARAMS", "extra": map[string]any{"validation_errors": []map[string]string{{"field": field, "message": message}}}})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusRequestEntityTooLarge)
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
		return
	}
	writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
}
