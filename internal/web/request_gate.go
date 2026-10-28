package web

import (
	"context"
	"errors"
)

var errRequestQueueFull = errors.New("request queue is full")

type requestGate struct {
	slots    chan struct{}
	waiters  chan struct{}
	maxQueue int
}

func newRequestGate(maxConcurrency, maxQueue int) *requestGate {
	if maxConcurrency <= 0 {
		return nil
	}
	if maxQueue < 0 {
		maxQueue = 0
	}
	return &requestGate{
		slots:    make(chan struct{}, maxConcurrency),
		waiters:  make(chan struct{}, maxQueue),
		maxQueue: maxQueue,
	}
}

func (g *requestGate) acquire(ctx context.Context) error {
	if g == nil {
		return nil
	}
	select {
	case g.slots <- struct{}{}:
		return nil
	default:
	}
	if g.maxQueue == 0 {
		return errRequestQueueFull
	}
	select {
	case g.waiters <- struct{}{}:
		defer func() { <-g.waiters }()
	default:
		return errRequestQueueFull
	}
	select {
	case g.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *requestGate) release() {
	if g != nil {
		<-g.slots
	}
}
