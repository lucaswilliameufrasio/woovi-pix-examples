package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/lucaseufrasio/woovi-pix-examples/reservas/backend/internal/store"
)

type API struct {
	Store          *store.Store
	SimulatorToken string
	OperatorToken  string
}

type apiError struct {
	Message   string         `json:"message"`
	ErrorCode string         `json:"error_code"`
	Extra     map[string]any `json:"extra,omitempty"`
}

func (a API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /v1/availability", a.availability)
	mux.HandleFunc("POST /v1/reservations", a.createHold)
	mux.HandleFunc("GET /v1/reservations/{reservation_id}", a.getPrivate)
	mux.HandleFunc("POST /v1/reservations/{reservation_id}/cancel", a.cancel)
	mux.HandleFunc("POST /v1/simulator/reservations/{reservation_id}/paid", a.simulatePaid)
	mux.HandleFunc("GET /v1/operator/reservations", a.operatorOnly(a.listOperator))
	mux.HandleFunc("POST /v1/operator/reservations/{reservation_id}/complete", a.operatorOnly(a.complete))
	return mux
}

func (a API) operatorOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !authorized(r, a.OperatorToken) {
			writeError(w, http.StatusUnauthorized, "OPERATOR_UNAUTHORIZED", "A autenticação do operador é inválida.")
			return
		}
		next(w, r)
	}
}

func (a API) listOperator(w http.ResponseWriter, r *http.Request) {
	reservations, err := a.Store.ListOperator(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, reservations)
}

func (a API) complete(w http.ResponseWriter, r *http.Request) {
	err := a.Store.Complete(r.Context(), r.PathValue("reservation_id"))
	if errors.Is(err, store.ErrInvalidState) {
		writeError(w, http.StatusConflict, "RESERVATION_NOT_COMPLETABLE", "A reserva ainda não pode ser concluída.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a API) availability(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "O parâmetro date deve usar YYYY-MM-DD.")
		return
	}
	slots, resource, err := a.Store.Availability(r.Context(), date)
	if err != nil {
		if errors.Is(err, store.ErrDateOutOfRange) {
			writeError(w, http.StatusUnprocessableEntity, "DATE_OUT_OF_RANGE", "A disponibilidade deve estar entre hoje e os próximos 14 dias.")
			return
		}
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "A data ou agenda solicitada é inválida.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{
		"resource_id": resource.ID, "resource_name": resource.Name, "time_zone": resource.TimeZone,
		"price_cents": resource.PriceCents, "duration_minutes": resource.DurationMinutes,
		"buffer_minutes": resource.BufferMinutes, "slots": slots,
	})
}

func (a API) createHold(w http.ResponseWriter, r *http.Request) {
	var input struct {
		StartsAt string `json:"starts_at"`
	}
	if err := decodeJSON(w, r, &input); err != nil {
		writeError(w, http.StatusBadRequest, "MALFORMED_REQUEST", "A requisição está malformada.")
		return
	}
	startsAt, err := time.Parse(time.RFC3339, input.StartsAt)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "INVALID_PARAMS", "starts_at deve ser um instante RFC3339 com fuso.")
		return
	}
	created, err := a.Store.CreateHold(r.Context(), startsAt)
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, http.StatusConflict, "SLOT_UNAVAILABLE", "O horário não está mais disponível.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusCreated, map[string]any{
		"reservation": created.Reservation,
		"capability":  created.Capability,
	})
}

func (a API) getPrivate(w http.ResponseWriter, r *http.Request) {
	reservation, err := a.Store.GetPrivate(r.Context(), r.PathValue("reservation_id"), bearer(r))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "RESERVATION_UNAUTHORIZED", "A credencial da reserva é inválida.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, reservation)
}

func (a API) cancel(w http.ResponseWriter, r *http.Request) {
	reservation, err := a.Store.Cancel(r.Context(), r.PathValue("reservation_id"), bearer(r))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusUnauthorized, "RESERVATION_UNAUTHORIZED", "A credencial da reserva é inválida.")
		return
	}
	if errors.Is(err, store.ErrInvalidState) {
		writeError(w, http.StatusConflict, "RESERVATION_NOT_CANCELLABLE", "A reserva não pode ser cancelada no estado atual.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	writeJSON(w, http.StatusOK, reservation)
}

func (a API) simulatePaid(w http.ResponseWriter, r *http.Request) {
	if !authorized(r, a.SimulatorToken) {
		writeError(w, http.StatusUnauthorized, "SIMULATOR_UNAUTHORIZED", "A autenticação do simulador é inválida.")
		return
	}
	eventKey := r.Header.Get("Idempotency-Key")
	if eventKey == "" {
		writeError(w, http.StatusBadRequest, "INVALID_PARAMS", "Idempotency-Key deve ser informado.")
		return
	}
	err := a.Store.SimulatePaid(r.Context(), r.PathValue("reservation_id"), eventKey)
	if errors.Is(err, store.ErrUnavailable) {
		writeError(w, http.StatusNotFound, "RESERVATION_NOT_FOUND", "Reserva não encontrada ou chave de evento conflitante.")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "UNEXPECTED_ERROR", "Ocorreu um erro inesperado.")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, destination any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("content type must be application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request contains trailing data")
	}
	return nil
}

func bearer(r *http.Request) string {
	const prefix = "Bearer "
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, prefix) {
		return ""
	}
	return value[len(prefix):]
}

func authorized(r *http.Request, expected string) bool {
	provided := bearer(r)
	return len(expected) >= 32 && len(provided) == len(expected) && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Message: message, ErrorCode: code})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}
