// Package app is the composition root: it wires adapters to the core services,
// owns their lifecycle and exposes the HTTP handler.
package app

import (
	"context"
	"sync"

	"github.com/ArsinShaabani/AIOS/internal/ports"
)

// Bus is a small in-process event bus. Subscribers run asynchronously so a slow
// listener can never block a chat turn.
type Bus struct {
	mu     sync.RWMutex
	subs   map[string][]func(context.Context, ports.Event)
	queue  chan ports.Event
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewBus builds the bus with a bounded buffer.
func NewBus() *Bus {
	return &Bus{subs: map[string][]func(context.Context, ports.Event){}, queue: make(chan ports.Event, 256)}
}

// Start launches the dispatcher.
func (b *Bus) Start(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	b.cancel = cancel
	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			case evt := <-b.queue:
				b.mu.RLock()
				handlers := append([]func(context.Context, ports.Event){}, b.subs[evt.Name]...)
				handlers = append(handlers, b.subs["*"]...)
				b.mu.RUnlock()
				for _, handler := range handlers {
					handler := handler
					b.wg.Add(1)
					go func() {
						defer b.wg.Done()
						handler(ctx, evt)
					}()
				}
			}
		}
	}()
}

// Publish queues an event, dropping it if the buffer is full.
func (b *Bus) Publish(_ context.Context, event ports.Event) {
	select {
	case b.queue <- event:
	default:
	}
}

// Subscribe registers a handler for an event name ("*" = all).
func (b *Bus) Subscribe(name string, handler func(context.Context, ports.Event)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[name] = append(b.subs[name], handler)
}

// Stop drains the dispatcher.
func (b *Bus) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
	b.wg.Wait()
}
