package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
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
	"MALFORMED_REQUEST":     "A requisição está malformada.",
	"UNEXPECTED_ERROR":      "Ocorreu um erro inesperado.",
	"PAYLOAD_TOO_LARGE":     "O corpo da requisição excede o limite permitido.",
	"DEPENDENCY_REQUEST":    "Não foi possível concluir a solicitação a uma dependência.",
	"INVALID_PARAMS":        "Parâmetros inválidos.",
	"OFFER_UNAVAILABLE":     "A oferta não está disponível.",
	"OFFER_NOT_FOUND":       "Oferta não encontrada.",
	"ORDER_NOT_FOUND":       "Pedido não encontrado.",
	"ORDER_NOT_ELIGIBLE":    "O pedido não permite esta operação no estado atual.",
	"OPERATOR_UNAUTHORIZED": "Autenticação do operador inválida.",
	"PICKUP_NOT_ELIGIBLE":   "O pedido ainda não está apto para retirada.",
	"PICKUP_ALREADY_DONE":   "A retirada deste pedido já foi concluída.",
	"PICKUP_TOKEN_INVALID":  "O código de retirada é inválido.",
	"IDEMPOTENCY_CONFLICT":  "A chave de idempotência já foi usada com outros dados.",
	"NOT_FOUND":             "Recurso não encontrado.",
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
	mux.Handle("GET /v1/operator/orders", a.operatorOnly(a.listOrders))
	mux.Handle("POST /v1/operator/orders/{order_id}/paid", a.operatorOnly(a.markOrderPaid))
	mux.Handle("POST /v1/operator/orders/{order_id}/pickup-token", a.operatorOnly(a.issuePickupToken))
	mux.Handle("POST /v1/operator/orders/{order_id}/pickup", a.operatorOnly(a.redeemPickup))
	mux.Handle("POST /v1/operator/demo/reset", a.operatorOnly(a.resetDemo))
	if os.Getenv("DEMO_MODE") == "true" {
		mux.HandleFunc("POST /v1/dev/webhooks/paid", a.paidWebhook)
	}
	return mux
}

func (a API) operatorOnly(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if os.Getenv("DEMO_MODE") != "true" {
			writeError(w, http.StatusNotFound, "NOT_FOUND")
			return
		}
		expected := os.Getenv("DEMO_OPERATOR_TOKEN")
		if len(expected) < 32 {
			writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
			return
		}
		const prefix = "Bearer "
		provided := r.Header.Get("Authorization")
		if len(provided) < len(prefix) || provided[:len(prefix)] != prefix || subtle.ConstantTimeCompare([]byte(provided[len(prefix):]), []byte(expected)) != 1 {
			writeError(w, http.StatusUnauthorized, "OPERATOR_UNAUTHORIZED")
			return
		}
		next.ServeHTTP(w, r)
	})
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
	if len(input.OfferID) > 200 {
		writeValidationError(w, "offer_id", "deve ter no máximo 200 caracteres")
		return
	}
	id, err := newID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	order, err := a.Store.CreateOrder(r.Context(), id, input.OfferID)
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, http.StatusPreconditionFailed, "OFFER_UNAVAILABLE")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "OFFER_NOT_FOUND")
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
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	orders, err := a.Store.ListOrders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (a API) issuePickupToken(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	token, err := a.Store.IssuePickupCredential(r.Context(), r.PathValue("order_id"))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if errors.Is(err, store.ErrPickupNotEligible) {
		writeError(w, http.StatusPreconditionFailed, "PICKUP_NOT_ELIGIBLE")
		return
	}
	if errors.Is(err, store.ErrPickupAlreadyCompleted) {
		writeError(w, http.StatusConflict, "PICKUP_ALREADY_DONE")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"pickup_token": token})
}

func (a API) redeemPickup(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	if err := requireJSON(r); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var input struct {
		PickupToken string `json:"pickup_token"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeDecodeError(w, err)
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeDecodeError(w, err)
		return
	}
	if input.PickupToken == "" {
		writeValidationError(w, "pickup_token", "deve ser informado")
		return
	}
	order, err := a.Store.RedeemPickupCredential(r.Context(), r.PathValue("order_id"), input.PickupToken)
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if errors.Is(err, store.ErrPickupNotEligible) {
		writeError(w, http.StatusPreconditionFailed, "PICKUP_NOT_ELIGIBLE")
		return
	}
	if errors.Is(err, store.ErrPickupAlreadyCompleted) {
		writeError(w, http.StatusConflict, "PICKUP_ALREADY_DONE")
		return
	}
	if errors.Is(err, store.ErrPickupCredentialInvalid) {
		writeError(w, http.StatusConflict, "PICKUP_TOKEN_INVALID")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a API) markOrderPaid(w http.ResponseWriter, r *http.Request) {
	if os.Getenv("DEMO_MODE") != "true" {
		writeError(w, http.StatusNotFound, "NOT_FOUND")
		return
	}
	order, err := a.Store.MarkPaid(r.Context(), r.PathValue("order_id"))
	var stateConflict store.ErrOrderStateConflict
	if errors.As(err, &stateConflict) {
		writeConflictError(w, "ORDER_NOT_ELIGIBLE", map[string]any{"order_state": stateConflict.State})
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func writePreconditionError(w http.ResponseWriter, code string, extra map[string]any) {
	message, ok := errorMessages[code]
	if !ok {
		code, message = "UNEXPECTED_ERROR", "Ocorreu um erro inesperado."
	}
	writeJSON(w, http.StatusPreconditionFailed, apiError{Message: message, ErrorCode: code, Extra: extra})
}

func writeConflictError(w http.ResponseWriter, code string, extra map[string]any) {
	message, ok := errorMessages[code]
	if !ok {
		code, message = "UNEXPECTED_ERROR", "Ocorreu um erro inesperado."
	}
	writeJSON(w, http.StatusConflict, apiError{Message: message, ErrorCode: code, Extra: extra})
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
	if len(in.OrderID) > 200 {
		writeValidationError(w, "order_id", "deve ter no máximo 200 caracteres")
		return
	}
	if len(in.EventKey) > 200 {
		writeValidationError(w, "event_key", "deve ter no máximo 200 caracteres")
		return
	}
	if _, err := a.Store.GetOrder(r.Context(), in.OrderID); errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
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
