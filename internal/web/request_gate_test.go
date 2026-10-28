package web

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRequestGateBoundsConcurrencyAndQueue(t *testing.T) {
	t.Parallel()
	gate := newRequestGate(1, 1)
	if err := gate.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}

	waiting := make(chan error, 1)
	go func() {
		waiting <- gate.acquire(context.Background())
	}()
	deadline := time.Now().Add(time.Second)
	for len(gate.waiters) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(gate.waiters) != 1 {
		t.Fatal("queued request did not enter the waiting queue")
	}
	if err := gate.acquire(context.Background()); !errors.Is(err, errRequestQueueFull) {
		t.Fatalf("third acquire error = %v, want queue full", err)
	}

	gate.release()
	if err := <-waiting; err != nil {
		t.Fatal(err)
	}
	gate.release()
}

func TestRequestGateHonorsContextCancellation(t *testing.T) {
	t.Parallel()
	gate := newRequestGate(1, 1)
	if err := gate.acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	waiting := make(chan error, 1)
	go func() {
		waiting <- gate.acquire(ctx)
	}()
	deadline := time.Now().Add(time.Second)
	for len(gate.waiters) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-waiting; !errors.Is(err, context.Canceled) {
		t.Fatalf("queued acquire error = %v, want context canceled", err)
	}
	gate.release()
}
