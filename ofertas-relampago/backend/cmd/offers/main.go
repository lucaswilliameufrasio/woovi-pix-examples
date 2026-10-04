package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/httpapi"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/simulator"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	storeURL := env("DATABASE_URL", "postgres://offers:offers-local-only@localhost:55431/offers?sslmode=disable")
	st, err := store.Open(ctx, storeURL)
	if err != nil {
		slog.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		slog.Error("database migration failed", "error", err)
		os.Exit(1)
	}
	apiServer := &http.Server{Handler: (httpapi.API{Store: st}).Handler(), ReadHeaderTimeout: 5 * time.Second}
	simServer := &http.Server{Handler: simulatorHandler(st), ReadHeaderTimeout: 5 * time.Second}
	apiListener, err := net.Listen("tcp", env("API_ADDR", "127.0.0.1:8080"))
	if err != nil {
		slog.Error("API listener failed", "error", err)
		os.Exit(1)
	}
	simListener, err := net.Listen("tcp", env("SIMULATOR_ADDR", "127.0.0.1:8081"))
	if err != nil {
		_ = apiListener.Close()
		slog.Error("simulator listener failed", "error", err)
		os.Exit(1)
	}
	apiAddress := apiListener.Addr().String()
	if err := os.Setenv("INTERNAL_API_ADDR", apiAddress); err != nil {
		_ = apiListener.Close()
		_ = simListener.Close()
		slog.Error("configure internal API address", "error", err)
		os.Exit(1)
	}
	errCh := make(chan error, 2)
	go func() { slog.Info("API listening", "addr", apiAddress); errCh <- apiServer.Serve(apiListener) }()
	go func() {
		slog.Info("simulator listening", "addr", simListener.Addr().String())
		errCh <- simServer.Serve(simListener)
	}()
	go runWorker(ctx, st)
	go runExpirer(ctx, st)
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			slog.Error("server failed", "error", err)
			os.Exit(1)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = apiServer.Shutdown(shutdownCtx)
	_ = simServer.Shutdown(shutdownCtx)
}

func simulatorHandler(st *store.Store) http.Handler {
	base := simulator.New(func(orderID string) (int64, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		o, err := st.GetOrder(ctx, orderID)
		return o.Amount, err
	}, func(orderID, eventKey string) error {
		body, _ := json.Marshal(map[string]string{"order_id": orderID, "event_key": eventKey})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+env("INTERNAL_API_ADDR", "127.0.0.1:8080")+"/dev/webhooks/paid", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusAccepted {
			return fmt.Errorf("webhook returned %s", resp.Status)
		}
		return nil
	})
	return base.Handler()
}

func runWorker(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			workerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := st.ProcessPaymentEvents(workerCtx, 32)
			cancel()
			if err != nil {
				slog.Error("payment event processing failed", "error", err)
			}
		}
	}
}

func runExpirer(ctx context.Context, st *store.Store) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			_, err := st.ExpireOrders(jobCtx)
			cancel()
			if err != nil {
				slog.Error("order expiration failed", "error", err)
			}
		}
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
