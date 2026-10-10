package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/lucaseufrasio/woovi-pix-examples/reservas/backend/internal/httpapi"
	"github.com/lucaseufrasio/woovi-pix-examples/reservas/backend/internal/store"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	databaseURL := os.Getenv("DATABASE_URL")
	simulatorToken := os.Getenv("DEMO_SIMULATOR_TOKEN")
	operatorToken := os.Getenv("DEMO_OPERATOR_TOKEN")
	address := os.Getenv("LISTEN_ADDR")
	if address == "" {
		address = "127.0.0.1:8084"
	}
	if databaseURL == "" || len(simulatorToken) < 32 || len(operatorToken) < 32 {
		return fmt.Errorf("DATABASE_URL, DEMO_SIMULATOR_TOKEN, and DEMO_OPERATOR_TOKEN (at least 32 characters each) are required")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid LISTEN_ADDR: %w", err)
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("LISTEN_ADDR must bind to a loopback IP address")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	st, err := store.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}
	server := &http.Server{Addr: address, Handler: httpapi.API{Store: st, SimulatorToken: simulatorToken, OperatorToken: operatorToken}.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 * 1024}
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				workCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
				if _, err := st.ProcessPaymentEvents(workCtx, 50); err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("reservation payment worker failed: %v", err)
				}
				if _, err := st.ExpireHolds(workCtx, 50); err != nil && !errors.Is(err, context.Canceled) {
					log.Printf("reservation expiry worker failed: %v", err)
				}
				cancel()
			}
		}
	}()
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			stop()
			return fmt.Errorf("serve reservations HTTP: %w", err)
		}
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown reservations HTTP: %w", err)
	}
	stop()
	workers.Wait()
	return nil
}
