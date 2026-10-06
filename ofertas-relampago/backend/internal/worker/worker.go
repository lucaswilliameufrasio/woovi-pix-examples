package worker

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

type paymentStore interface {
	ProcessPaymentEvents(context.Context, int) (int, error)
}

type expiryStore interface {
	ExpireOrders(context.Context) (int64, error)
}

type Store interface {
	paymentStore
	expiryStore
}

func Run(ctx context.Context, st Store, logger *slog.Logger) {
	if logger == nil {
		logger = slog.Default()
	}
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		runLoop(ctx, 250*time.Millisecond, 5*time.Second, logger, "payment event processing failed", func(jobCtx context.Context) error {
			_, err := st.ProcessPaymentEvents(jobCtx, 32)
			return err
		})
	}()
	go func() {
		defer workers.Done()
		runLoop(ctx, time.Second, 5*time.Second, logger, "order expiration failed", func(jobCtx context.Context) error {
			_, err := st.ExpireOrders(jobCtx)
			return err
		})
	}()
	logger.Info("background workers started")
	<-ctx.Done()
	workers.Wait()
}

func runLoop(ctx context.Context, interval, timeout time.Duration, logger *slog.Logger, message string, job func(context.Context) error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			jobCtx, cancel := context.WithTimeout(ctx, timeout)
			err := job(jobCtx)
			cancel()
			if err != nil && ctx.Err() == nil {
				logger.Error(message, "error", err)
			}
		}
	}
}
