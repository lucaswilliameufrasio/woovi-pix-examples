package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/click-collect/backend/internal/store"
)

type API struct {
	store          *store.Store
	operatorToken  string
	simulatorToken string
}

type apiError struct {
	Message   string `json:"message"`
	ErrorCode string `json:"error_code"`
}

func New(st *store.Store, operatorToken, simulatorToken string) http.Handler {
	api := &API{store: st, operatorToken: operatorToken, simulatorToken: simulatorToken}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("GET /v1/products", api.listProducts)
	mux.HandleFunc("POST /v1/orders", api.createOrder)
	mux.HandleFunc("GET /v1/orders/{order_id}", api.getOrder)
	mux.HandleFunc("DELETE /v1/orders/{order_id}", api.cancelOrder)
	mux.HandleFunc("POST /v1/orders/{order_id}/demo-payment", api.simulateAuthorizedPayment)
	mux.HandleFunc("POST /v1/simulator/orders/{order_id}/charge", api.createCharge)
	mux.HandleFunc("POST /v1/simulator/orders/{order_id}/confirm", api.confirmCharge)
	mux.HandleFunc("GET /v1/operator/orders", api.operatorQueue)
	mux.HandleFunc("POST /v1/operator/orders/{order_id}/ready", api.markReady)
	mux.HandleFunc("POST /v1/operator/orders/{order_id}/pickup", api.redeemPickup)
	return securityHeaders(mux)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.store.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "UNAVAILABLE", "Serviço indisponível.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listProducts(w http.ResponseWriter, r *http.Request) {
	products, err := a.store.ListProducts(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, products)
}

type reserveRequest struct {
	ProductID string `json:"product_id"`
}
type reservedOrder struct {
	store.Order
	OrderAccessToken string `json:"order_access_token"`
	PickupCode       string `json:"pickup_code"`
}

func (a *API) createOrder(w http.ResponseWriter, r *http.Request) {
	var request reserveRequest
	if !decodeJSON(w, r, &request) || strings.TrimSpace(request.ProductID) == "" {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST", "A requisição é inválida.")
		return
	}
	id, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	access, err := randomHex(32)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	pickup, err := randomPickupCode()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	order, err := a.store.Reserve(r.Context(), id, request.ProductID, digest(access), digest(pickup))
	if errors.Is(err, store.ErrProductNotFound) {
		writeError(w, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produto não encontrado.")
		return
	}
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, http.StatusConflict, "PRODUCT_UNAVAILABLE", "Produto sem unidades disponíveis.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusCreated, reservedOrder{Order: order, OrderAccessToken: access, PickupCode: pickup})
}

func (a *API) getOrder(w http.ResponseWriter, r *http.Request) {
	access, ok := bearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "ORDER_ACCESS_REQUIRED", "Acesso ao pedido necessário.")
		return
	}
	order, err := a.store.GetOrder(r.Context(), r.PathValue("order_id"), digest(access))
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) cancelOrder(w http.ResponseWriter, r *http.Request) {
	access, ok := bearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "ORDER_ACCESS_REQUIRED", "Acesso ao pedido necessário.")
		return
	}
	order, err := a.store.Cancel(r.Context(), r.PathValue("order_id"), digest(access))
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if errors.Is(err, store.ErrOrderConflict) {
		writeError(w, http.StatusConflict, "ORDER_NOT_CANCELLABLE", "O pedido não pode ser cancelado.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func (a *API) simulateAuthorizedPayment(w http.ResponseWriter, r *http.Request) {
	access, ok := bearer(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "ORDER_ACCESS_REQUIRED", "Acesso ao pedido necessário.")
		return
	}
	err := a.store.SimulateAuthorizedPayment(r.Context(), r.PathValue("order_id"), digest(access))
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if errors.Is(err, store.ErrOrderConflict) {
		writeError(w, http.StatusConflict, "ORDER_NOT_PAYABLE", "O pedido não pode receber uma confirmação simulada.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func (a *API) createCharge(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.simulatorToken) {
		writeError(w, http.StatusUnauthorized, "SIMULATOR_AUTH_REQUIRED", "Autorização do simulador necessária.")
		return
	}
	chargeID, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	eventKey, err := randomHex(16)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	charge, err := a.store.CreateSimulatedCharge(r.Context(), r.PathValue("order_id"), "sim_"+chargeID, "create_"+eventKey)
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if errors.Is(err, store.ErrOrderConflict) {
		writeError(w, http.StatusConflict, "ORDER_NOT_PAYABLE", "O pedido não pode receber uma cobrança de demonstração.")
		return
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusCreated, charge)
}

func (a *API) confirmCharge(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.simulatorToken) {
		writeError(w, http.StatusUnauthorized, "SIMULATOR_AUTH_REQUIRED", "Autorização do simulador necessária.")
		return
	}
	eventKey := "paid_" + r.PathValue("order_id")
	if err := a.store.ConfirmSimulatedCharge(r.Context(), r.PathValue("order_id"), eventKey); errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "CHARGE_NOT_FOUND", "Cobrança de demonstração não encontrada.")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (a *API) operatorQueue(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.operatorToken) {
		writeError(w, http.StatusUnauthorized, "OPERATOR_AUTH_REQUIRED", "Autorização da loja necessária.")
		return
	}
	orders, err := a.store.OperatorQueue(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, orders)
}

func (a *API) markReady(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.operatorToken) {
		writeError(w, http.StatusUnauthorized, "OPERATOR_AUTH_REQUIRED", "Autorização da loja necessária.")
		return
	}
	order, err := a.store.SetFulfillment(r.Context(), r.PathValue("order_id"), "ready_for_pickup")
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if errors.Is(err, store.ErrOrderConflict) {
		writeError(w, http.StatusConflict, "FULFILLMENT_NOT_ALLOWED", "A transição de preparo não é permitida.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

type pickupRequest struct {
	PickupCode string `json:"pickup_code"`
}

func (a *API) redeemPickup(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.operatorToken) {
		writeError(w, http.StatusUnauthorized, "OPERATOR_AUTH_REQUIRED", "Autorização da loja necessária.")
		return
	}
	var request pickupRequest
	if !decodeJSON(w, r, &request) || len(request.PickupCode) != 8 {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST", "O código de retirada é inválido.")
		return
	}
	order, err := a.store.RedeemPickup(r.Context(), r.PathValue("order_id"), digest(request.PickupCode))
	if errors.Is(err, store.ErrOrderNotFound) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND", "Pedido não encontrado.")
		return
	}
	if errors.Is(err, store.ErrPickupCodeInvalid) {
		writeError(w, http.StatusForbidden, "PICKUP_CODE_INVALID", "O código de retirada é inválido.")
		return
	}
	if errors.Is(err, store.ErrPickupAlreadyDone) {
		writeError(w, http.StatusConflict, "PICKUP_ALREADY_COMPLETED", "A retirada já foi concluída.")
		return
	}
	if errors.Is(err, store.ErrPickupNotEligible) {
		writeError(w, http.StatusConflict, "PICKUP_NOT_READY", "O pedido ainda não está pronto para retirada.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, order)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 16*1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return false
	}
	var extra any
	return errors.Is(decoder.Decode(&extra), io.EOF)
}

func bearer(r *http.Request) (string, bool) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) != 64 {
		return "", false
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", false
	}
	return parts[1], true
}

func authorized(r *http.Request, expected string) bool {
	provided := r.Header.Get("X-Demo-Token")
	if expected == "" || len(provided) != len(expected) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate random credential: %w", err)
	}
	return hex.EncodeToString(value), nil
}

func randomPickupCode() (string, error) {
	value, err := randomHex(4)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(value), nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Message: message, ErrorCode: code})
}
