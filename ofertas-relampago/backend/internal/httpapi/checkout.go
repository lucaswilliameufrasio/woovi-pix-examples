package httpapi

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func privateCheckout(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (a API) issueLocalCheckout(w http.ResponseWriter, r *http.Request) {
	a.issueCheckout(w, r, false)
}

func (a API) issueCustomerCheckout(w http.ResponseWriter, r *http.Request) {
	a.issueCheckout(w, r, true)
}

func (a API) issueCheckout(w http.ResponseWriter, r *http.Request, customer bool) {
	orderToken := ""
	if customer {
		orderToken = checkoutBearer(r)
		if orderToken == "" {
			writeError(w, http.StatusUnauthorized, "ORDER_UNAUTHORIZED")
			return
		}
	}
	if len(r.Header.Values("Idempotency-Key")) != 1 || !validCheckoutKey(r.Header.Get("Idempotency-Key")) {
		writeValidationError(w, "idempotency_key", "informe uma chave ASCII de 16 a 128 caracteres, sem espaços")
		return
	}
	orderID := r.PathValue("order_id")
	if len(orderID) == 0 || len(orderID) > 200 || strings.TrimSpace(orderID) != orderID {
		writeValidationError(w, "order_id", "informe um identificador de pedido válido")
		return
	}
	// Path and key are the entire command. Reject client prices or identity.
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		writeDecodeError(w, err)
		return
	}
	if len(body) != 0 {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST")
		return
	}
	var grant store.LocalCheckoutGrant
	var created bool
	if customer {
		grant, created, err = a.Store.IssueCustomerLocalCheckout(r.Context(), orderID, r.Header.Get("Idempotency-Key"), orderToken)
	} else {
		grant, created, err = a.Store.IssueLocalCheckout(r.Context(), orderID, r.Header.Get("Idempotency-Key"))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "ORDER_NOT_FOUND")
		return
	}
	if err != nil {
		writeLocalCheckoutError(w, err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, struct {
		store.LocalCheckoutGrant
		Currency string `json:"currency"`
		Mode     string `json:"mode"`
	}{LocalCheckoutGrant: grant, Currency: "BRL", Mode: "local_simulation"})
}

func validCheckoutKey(key string) bool {
	if len(key) < 16 || len(key) > 128 {
		return false
	}
	for _, c := range key {
		if c < '!' || c > '~' {
			return false
		}
	}
	return true
}

func (a API) getLocalCheckout(w http.ResponseWriter, r *http.Request) {
	// This endpoint remains local-demo-only even if used by an SDK transport.
	if len(r.Header.Values("Authorization")) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeError(w, http.StatusUnauthorized, "CHECKOUT_UNAUTHORIZED")
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	c, err := a.Store.GetLocalCheckout(r.Context(), r.PathValue("checkout_id"), token)
	if err != nil {
		writeLocalCheckoutError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		store.LocalCheckout
		Currency string `json:"currency"`
		Mode     string `json:"mode"`
	}{LocalCheckout: c, Currency: "BRL", Mode: "local_simulation"})
}

func writeLocalCheckoutError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrOrderAccessUnauthorized):
		writeError(w, http.StatusUnauthorized, "ORDER_UNAUTHORIZED")
	case errors.Is(err, store.ErrCheckoutUnauthorized):
		writeError(w, http.StatusUnauthorized, "CHECKOUT_UNAUTHORIZED")
	case errors.Is(err, store.ErrCheckoutKeyConflict), errors.Is(err, store.ErrChargeConflict):
		writeError(w, http.StatusConflict, "IDEMPOTENCY_CONFLICT")
	case errors.Is(err, store.ErrCheckoutNotEligible):
		writeError(w, http.StatusPreconditionFailed, "CHECKOUT_NOT_ELIGIBLE")
	case errors.Is(err, store.ErrCheckoutPaymentException):
		writePreconditionError(w, "PAYMENT_EXCEPTION", map[string]any{"order_state": "payment_exception"})
	default:
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR")
	}
}

func checkoutBearer(r *http.Request) string {
	if len(r.Header.Values("Authorization")) != 1 || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return ""
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if len(token) != 64 {
		return ""
	}
	return token
}

func (a API) getCustomerOrder(w http.ResponseWriter, r *http.Request) {
	order, err := a.Store.GetCustomerOrder(r.Context(), r.PathValue("order_id"), checkoutBearer(r))
	if err != nil {
		writeLocalCheckoutError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, order)
}
