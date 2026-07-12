package mcp

import "sync"

// notifications.go provides tool list change notifications via publish-subscribe.

// toolListChangedNotifier publishes tool-list-changed notifications to subscribers.
type toolListChangedNotifier struct {
	mu          sync.RWMutex
	subscribers []chan struct{}
}

// newToolListChangedNotifier creates a new notifier.
func newToolListChangedNotifier() *toolListChangedNotifier {
	return &toolListChangedNotifier{
		subscribers: make([]chan struct{}, 0),
	}
}

// Subscribe registers a subscriber and returns its notification channel.
func (n *toolListChangedNotifier) Subscribe() chan struct{} {
	n.mu.Lock()
	defer n.mu.Unlock()
	ch := make(chan struct{}, 1)
	n.subscribers = append(n.subscribers, ch)
	return ch
}

// Unsubscribe removes a subscriber by its channel reference.
func (n *toolListChangedNotifier) Unsubscribe(ch chan struct{}) {
	n.mu.Lock()
	defer n.mu.Unlock()
	for i, sub := range n.subscribers {
		if sub == ch {
			n.subscribers = append(n.subscribers[:i], n.subscribers[i+1:]...)
			return
		}
	}
}

// Notify signals all subscribers that the tool list has changed.
// Non-blocking: drops the signal if the subscriber's buffer is full.
func (n *toolListChangedNotifier) Notify() {
	n.mu.RLock()
	defer n.mu.RUnlock()
	for _, sub := range n.subscribers {
		select {
		case sub <- struct{}{}:
		default:
		}
	}
}

// subscriberCount returns the current number of subscribers (for tests).
func (n *toolListChangedNotifier) subscriberCount() int {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return len(n.subscribers)
}
