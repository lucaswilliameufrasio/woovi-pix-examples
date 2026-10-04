package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

type API struct{ Store *store.Store }

type apiError struct {
	Message   string         `json:"message"`
	ErrorCode string         `json:"error_code"`
	Extra     map[string]any `json:"extra,omitempty"`
}

var errorMessages = map[string]string{
	"MALFORMED_REQUEST":    "A requisição está malformada.",
	"UNEXPECTED_ERROR":     "Ocorreu um erro inesperado.",
	"PAYLOAD_TOO_LARGE":    "O corpo da requisição excede o limite permitido.",
	"DEPENDENCY_REQUEST":   "Não foi possível concluir a solicitação a uma dependência.",
	"INVALID_PARAMS":       "Parâmetros inválidos.",
	"OFFER_UNAVAILABLE":    "A oferta não está disponível.",
	"ORDER_NOT_FOUND":      "Pedido não encontrado.",
	"IDEMPOTENCY_CONFLICT": "A chave de idempotência já foi usada com outros dados.",
	"NOT_FOUND":            "Recurso não encontrado.",
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/offers", func(w http.ResponseWriter, r *http.Request) {
		offers, err := a.Store.ListOffers(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
			return
		}
		writeJSON(w, http.StatusOK, offers)
	})
	mux.HandleFunc("POST /v1/orders", a.createOrder)
	mux.HandleFunc("GET /v1/orders/{order_id}", a.getOrder)
	mux.HandleFunc("GET /v1/operator/orders", a.listOrders)
	mux.HandleFunc("POST /v1/operator/demo/reset", a.resetDemo)
	if os.Getenv("DEMO_MODE") == "true" {
		mux.HandleFunc("POST /v1/dev/webhooks/paid", a.paidWebhook)
	}
	return mux
}

func (a API) createOrder(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OfferID string `json:"offer_id"`
	}
	if err := requireJSON(r); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&input); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if input.OfferID == "" {
		writeValidationError(w, "offer_id", "deve ser informado")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	order, err := a.Store.CreateOrder(r.Context(), id, input.OfferID)
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, http.StatusConflict, "OFFER_UNAVAILABLE")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, order)
}

func (a API) getOrder(w http.ResponseWriter, r *http.Request) {
	o, err := a.Store.GetOrder(r.Context(), r.PathValue("order_id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, o)
}
func (a API) listOrders(w http.ResponseWriter, r *http.Request) {
	orders, err := a.Store.ListOrders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, orders)
}
func (a API) resetDemo(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err := a.Store.ResetDemo(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (a API) paidWebhook(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err := requireJSON(r); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var in struct {
		OrderID  string `json:"order_id"`
		EventKey string `json:"event_key"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if in.OrderID == "" || in.EventKey == "" {
		field := "order_id"
		if in.OrderID != "" {
			field = "event_key"
		}
		writeValidationError(w, field, "deve ser informado")
		return
	}
	if _, err := a.Store.GetOrder(r.Context(), in.OrderID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		} else {
			writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		}
		return
	}
	sum := sha256.Sum256([]byte(in.EventKey))
	eventID := hex.EncodeToString(sum[:])
	_, err := a.Store.PersistPaidEvent(r.Context(), eventID, in.OrderID, in.EventKey)
	if errors.Is(err, store.ErrEventKeyConflict) {
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
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
	message, ok := errorMessages[code]
	if !ok || status >= 500 && status != http.StatusBadGateway && status != http.StatusGatewayTimeout {
		code, message = "UNEXPECTED_ERROR", "Ocorreu um erro inesperado."
	}
	writeJSON(w, status, apiError{Message: message, ErrorCode: code})
}

func writeValidationError(w http.ResponseWriter, field, message string) {
	writeJSON(w, http.StatusUnprocessableEntity, apiError{
		Message: "Parâmetros inválidos.", ErrorCode: "INVALID_PARAMS",
		Extra: map[string]any{"validation_errors": []map[string]string{{"field": field, "message": message}}},
	})
}

func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		writeError(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE")
		return
	}
	writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
}

func requireJSON(r *http.Request) error {
	contentType := r.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "application/json" {
		return errors.New("request content type must be application/json")
	}
	return nil
}
