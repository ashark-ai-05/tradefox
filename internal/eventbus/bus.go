package eventbus

import (
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/ashark-ai-05/tradefox/internal/core/models"
)

// TopicBus is a generic pub/sub bus for a single topic.
// Subscribers receive events via channels. Publishing fans out to all subscribers.
type TopicBus[T any] struct {
	mu          sync.RWMutex
	subscribers map[uint64]chan T
	nextID      atomic.Uint64
	closed      atomic.Bool
	logger      *slog.Logger
}

// NewTopicBus creates a new topic bus.
func NewTopicBus[T any](logger *slog.Logger) *TopicBus[T] {
	return &TopicBus[T]{
		subscribers: make(map[uint64]chan T),
		logger:      logger,
	}
}

// Subscribe registers a subscriber. Returns a unique ID and a receive-only channel.
// bufSize controls the subscriber's channel buffer. If the subscriber is slow and
// the buffer fills, events are dropped (non-blocking send) to protect other subscribers.
func (b *TopicBus[T]) Subscribe(bufSize int) (id uint64, ch <-chan T) {
	if bufSize < 1 {
		bufSize = 1
	}

	id = b.nextID.Add(1)
	c := make(chan T, bufSize)

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed.Load() {
		close(c)
		return id, c
	}

	b.subscribers[id] = c
	return id, c
}

// Unsubscribe removes a subscriber by ID and closes its channel.
func (b *TopicBus[T]) Unsubscribe(id uint64) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

// Publish sends an event to all subscribers. Non-blocking: if a subscriber's
// buffer is full, the event is dropped for that subscriber (logged as warning).
func (b *TopicBus[T]) Publish(event T) {
	if b.closed.Load() {
		return
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for id, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
			b.logger.Warn("event dropped: subscriber buffer full",
				slog.Uint64("subscriber_id", id),
			)
		}
	}
}

// PublishBatch sends multiple events to all subscribers.
func (b *TopicBus[T]) PublishBatch(events []T) {
	for _, event := range events {
		b.Publish(event)
	}
}

// SubscriberCount returns the current number of subscribers.
func (b *TopicBus[T]) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers)
}

// Close unsubscribes all subscribers and marks the bus as closed.
func (b *TopicBus[T]) Close() {
	b.closed.Store(true)

	b.mu.Lock()
	defer b.mu.Unlock()

	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}
}

// ExecutionEvent represents an event from the execution engine.
type ExecutionEvent struct {
	Type string      `json:"type"` // "order:placed", "order:filled", "order:cancelled", "position:update", "risk:alert"
	Data interface{} `json:"data"`
}

// Bus holds all topic buses for the system.
// OrderBooks uses *models.OrderBook (pointer) because OrderBook contains
// sync.RWMutex and atomic fields that must not be copied by value.
type Bus struct {
	OrderBooks *TopicBus[*models.OrderBook]
	Trades     *TopicBus[models.Trade]
	Providers  *TopicBus[models.Provider]
	Positions  *TopicBus[models.Order]
	Studies    *TopicBus[models.BaseStudyModel]
	Execution  *TopicBus[ExecutionEvent]
}

// NewBus creates a Bus with all topic buses initialized.
func NewBus(logger *slog.Logger) *Bus {
	return &Bus{
		OrderBooks: NewTopicBus[*models.OrderBook](logger),
		Trades:     NewTopicBus[models.Trade](logger),
		Providers:  NewTopicBus[models.Provider](logger),
		Positions:  NewTopicBus[models.Order](logger),
		Studies:    NewTopicBus[models.BaseStudyModel](logger),
		Execution:  NewTopicBus[ExecutionEvent](logger),
	}
}

// Close closes all topic buses.
func (b *Bus) Close() {
	b.OrderBooks.Close()
	b.Trades.Close()
	b.Providers.Close()
	b.Positions.Close()
	b.Studies.Close()
	b.Execution.Close()
}
