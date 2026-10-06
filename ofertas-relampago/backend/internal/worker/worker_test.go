package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

type idleStore struct{}

func (idleStore) ProcessPaymentEvents(context.Context, int) (int, error) { return 0, nil }
func (idleStore) ExpireOrders(context.Context) (int64, error)            { return 0, nil }

func TestRunStopsOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		Run(ctx, idleStore{}, logger)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker loops did not stop after context cancellation")
	}
}
