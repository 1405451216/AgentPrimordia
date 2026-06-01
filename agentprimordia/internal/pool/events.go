package pool

import (
	"context"
	"sync"
)

type EventBus struct {
	mu     sync.RWMutex
	subs   map[chan<- PoolEvent]struct{}
	closed bool
}

func NewEventBus() *EventBus {
	return &EventBus{
		subs: make(map[chan<- PoolEvent]struct{}),
	}
}

func (eb *EventBus) Subscribe(ch chan<- PoolEvent) func() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	if eb.closed {
		return func() {}
	}

	eb.subs[ch] = struct{}{}

	return func() {
		eb.Unsubscribe(ch)
	}
}

func (eb *EventBus) Unsubscribe(ch chan<- PoolEvent) {
	eb.mu.Lock()
	defer eb.mu.Unlock()
	delete(eb.subs, ch)
}

func (eb *EventBus) Publish(event PoolEvent) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	if eb.closed {
		return
	}

	for ch := range eb.subs {
		select {
		case ch <- event:
		default:
		}
	}
}

func (eb *EventBus) Close() {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.closed = true

	for ch := range eb.subs {
		close(ch)
	}

	eb.subs = make(map[chan<- PoolEvent]struct{})
}

func (eb *EventBus) Watch(ctx context.Context, filter func(PoolEvent) bool) ([]PoolEvent, error) {
	ch := make(chan PoolEvent, 100)
	unsub := eb.Subscribe(ch)
	defer unsub()

	var events []PoolEvent

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return events, nil
			}

			if filter == nil || filter(event) {
				events = append(events, event)
			}

		case <-ctx.Done():
			return events, ctx.Err()
		}
	}
}

type EventCollector struct {
	bus       *EventBus
	events    []PoolEvent
	mu        sync.Mutex
	condition func([]PoolEvent) bool
	done      chan struct{}
	stopOnce  sync.Once
}

func NewEventCollector(bus *EventBus, condition func([]PoolEvent) bool) *EventCollector {
	return &EventCollector{
		bus:       bus,
		events:    make([]PoolEvent, 0),
		condition: condition,
		done:      make(chan struct{}),
	}
}

func (ec *EventCollector) Start(ctx context.Context) (<-chan struct{}, error) {
	ch := make(chan PoolEvent, 100)
	unsub := ec.bus.Subscribe(ch)

	go func() {
		defer ec.stopOnce.Do(func() { close(ec.done) })
		defer unsub()

		for {
			select {
			case event, ok := <-ch:
				if !ok {
					return
				}

				ec.mu.Lock()
				ec.events = append(ec.events, event)

				if ec.condition != nil && ec.condition(ec.events) {
					ec.mu.Unlock()
					return
				}
				ec.mu.Unlock()

			case <-ctx.Done():
				return
			case <-ec.done:
				return
			}
		}
	}()

	return ec.done, nil
}

func (ec *EventCollector) Events() []PoolEvent {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	return ec.events
}

func (ec *EventCollector) Stop() {
	ec.stopOnce.Do(func() {
		close(ec.done)
	})
}
