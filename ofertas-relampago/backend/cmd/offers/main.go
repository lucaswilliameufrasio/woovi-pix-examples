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
	"syscall"
	"time"

	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/httpapi"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/simulator"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
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
	if os.Getenv("DEMO_MODE") != "true" {
		slog.Error("this backend currently supports the local simulator only; set DEMO_MODE=true for local use")
		os.Exit(1)
	}
	if len(os.Getenv("DEMO_OPERATOR_TOKEN")) < 32 {
		slog.Error("DEMO_OPERATOR_TOKEN must contain at least 32 characters; generate and configure a local secret")
		os.Exit(1)
	}
	apiServer := &http.Server{Handler: (httpapi.API{Store: st}).Handler(), ReadHeaderTimeout: 5 * time.Second}
	simServer := &http.Server{Handler: simulatorHandler(st), ReadHeaderTimeout: 5 * time.Second}
	apiAddr := env("API_ADDR", "127.0.0.1:8080")
	if !loopbackAddress(apiAddr) {
		slog.Error("API_ADDR must use a loopback IP literal")
		os.Exit(1)
	}
	simAddr := env("SIMULATOR_ADDR", "127.0.0.1:8081")
	if !loopbackAddress(simAddr) {
		slog.Error("SIMULATOR_ADDR must use a loopback IP literal")
		os.Exit(1)
	}
	apiListener, err := net.Listen("tcp", apiAddr)
	if err != nil {
		slog.Error("API listener failed", "error", err)
		os.Exit(1)
	}
	simListener, err := net.Listen("tcp", simAddr)
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
	base := simulator.New(st, func(orderID string) (int64, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		o, err := st.GetOrder(ctx, orderID)
		return o.Amount, err
	}, func(orderID, eventKey string) error {
		body, _ := json.Marshal(map[string]string{"order_id": orderID, "event_key": eventKey})
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+env("INTERNAL_API_ADDR", "127.0.0.1:8080")+"/v1/dev/webhooks/paid", bytes.NewReader(body))
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

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
