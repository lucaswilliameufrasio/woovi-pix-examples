package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/store"
	"github.com/lucaseufrasio/woovi-pix-examples/ofertas-relampago/backend/internal/worker"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("DEMO_MODE") != "true" {
		return errors.New("this worker currently supports the local simulator only; set DEMO_MODE=true for local use")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://offers:offers-local-only@localhost:55431/offers?sslmode=disable"
	}
	st, err := store.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	migrationCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err = st.Migrate(migrationCtx)
	cancel()
	if err != nil {
		return err
	}
	worker.Run(ctx, st, slog.Default())
	return nil
}
