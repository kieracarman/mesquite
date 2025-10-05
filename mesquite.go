// Package mesquite provides a high-performance, observable message queue
// for Go applications with real-time monitoring capabilities.
package mesquite

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// Message represents a single message in the queue
type Message struct {
	ID        string
	Topic     string
	Payload   any
	Timestamp time.Time
	Attempts  int
	metadata  map[string]any
}

// Config holds configuration for the message queue
type Config struct {
	// MaxQueueSize is the maximum number of messages per topic queue
	MaxQueueSize int
	// WorkerCount is the number of concurrent workers processing messages
	WorkerCount int
	// MaxRetries is the maximum number of retry attempts for failed messages
	MaxRetries int
	// RetryDelay is the delay between retry attempts
	RetryDelay time.Duration
	// BatchSize is the number of messages to process in a batch
	BatchSize int
	// BatchTimeout is the maximum time to wait for a full batch
	BatchTimeout time.Duration
	// ShutdownTimeout is the maximum time to wait for graceful shutdown
	ShutdownTimeout time.Duration
	// EnableDashboard enables the real-time monitoring dashboard
	EnableDashboard bool
	// DashboardRefreshRate is how often the dashboard updates
	DashboardRefreshRate time.Duration
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() Config {
	return Config{
		MaxQueueSize:         10000,
		WorkerCount:          10,
		MaxRetries:           3,
		RetryDelay:           time.Second,
		BatchSize:            100,
		BatchTimeout:         100 * time.Millisecond,
		ShutdownTimeout:      30 * time.Second,
		EnableDashboard:      false,
		DashboardRefreshRate: 500 * time.Millisecond,
	}
}

// Queue is the main message queue structure
type Queue struct {
	config      Config
	queues      map[string]chan Message
	workers     []*worker
	handlers    map[string]HandlerFunc
	mu          sync.RWMutex
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	stats       *Stats
	subscribers map[string][]chan Message
	dashboard   *Dashboard
}

// HandlerFunc is a function that processes a message
// Return an error to trigger a retry
type HandlerFunc func(ctx context.Context, msg Message) error

// New creates a new message queue with the given configuration
func New(config Config) *Queue {
	ctx, cancel := context.WithCancel(context.Background())

	q := &Queue{
		config:      config,
		queues:      make(map[string]chan Message),
		workers:     make([]*worker, 0, config.WorkerCount),
		handlers:    make(map[string]HandlerFunc),
		ctx:         ctx,
		cancel:      cancel,
		stats:       newStats(config.WorkerCount),
		subscribers: make(map[string][]chan Message),
	}

	if config.EnableDashboard {
		q.dashboard = newDashboard(q)
		go q.startDashboard()
	}

	q.addEvent("SYSTEM", "Message queue initialized")
	q.startWorkers()

	return q
}

// NewWithDefaults creates a new message queue with default configuration
func NewWithDefaults() *Queue {
	return New(DefaultConfig())
}

// Publish sends a message to a specific topic
func (q *Queue) Publish(topic string, payload any) error {
	return q.PublishWithContext(q.ctx, topic, payload)
}

// PublishWithContext sends a message to a specific topic with a context
func (q *Queue) PublishWithContext(ctx context.Context, topic string, payload any) error {
	q.mu.RLock()
	queue, exists := q.queues[topic]
	q.mu.RUnlock()

	if !exists {
		q.mu.Lock()
		queue = make(chan Message, q.config.MaxQueueSize)
		q.queues[topic] = queue
		q.stats.mu.Lock()
		q.stats.QueueSize[topic] = 0
		q.stats.mu.Unlock()
		q.mu.Unlock()
		q.addEvent("QUEUE", fmt.Sprintf("New topic: %s", topic))
	}

	if len(queue) >= q.config.MaxQueueSize {
		q.addEvent("ERROR", fmt.Sprintf("Queue full: %s", topic))
		return fmt.Errorf("queue for topic %s is full", topic)
	}

	msg := Message{
		ID:        generateID(),
		Topic:     topic,
		Payload:   payload,
		Timestamp: time.Now(),
		Attempts:  0,
		metadata:  make(map[string]any),
	}

	select {
	case queue <- msg:
		atomic.AddUint64(&q.stats.MessagesQueued, 1)
		q.stats.mu.Lock()
		q.stats.QueueSize[topic]++
		q.stats.mu.Unlock()

		q.notifySubscribers(topic, msg)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-q.ctx.Done():
		return fmt.Errorf("queue is shutting down")
	}
}

// RegisterHandler registers a handler function for a specific topic
func (q *Queue) RegisterHandler(topic string, handler HandlerFunc) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.handlers[topic] = handler

	if _, exists := q.queues[topic]; !exists {
		q.queues[topic] = make(chan Message, q.config.MaxQueueSize)
		q.stats.mu.Lock()
		q.stats.QueueSize[topic] = 0
		q.stats.mu.Unlock()
	}

	q.addEvent("HANDLER", fmt.Sprintf("Registered: %s", topic))
}

// Subscribe creates a subscription to a topic
// Returns a channel that receives copies of messages published to the topic
func (q *Queue) Subscribe(topic string) <-chan Message {
	ch := make(chan Message, q.config.BatchSize)

	q.mu.Lock()
	q.subscribers[topic] = append(q.subscribers[topic], ch)
	subscriberCount := len(q.subscribers[topic])
	q.mu.Unlock()

	q.addEvent("SUBSCRIBE", fmt.Sprintf("Topic: %s (#%d)", topic, subscriberCount))

	return ch
}

// notifySubscribers sends a message to all subscribers of a topic
func (q *Queue) notifySubscribers(topic string, msg Message) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	subscribers, exists := q.subscribers[topic]
	if !exists || len(subscribers) == 0 {
		return
	}

	for _, sub := range subscribers {
		select {
		case sub <- msg:
		default:
			// Skip if subscriber is busy
		}
	}
}

// GetStats returns current queue statistics
func (q *Queue) GetStats() StatsSnapshot {
	return q.stats.Snapshot()
}

// Shutdown gracefully shuts down the message queue
func (q *Queue) Shutdown() error {
	q.addEvent("SYSTEM", "Shutdown initiated")
	q.cancel()

	for _, worker := range q.workers {
		close(worker.stop)
	}

	done := make(chan struct{})
	go func() {
		q.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		q.addEvent("SYSTEM", "Shutdown complete")
		return nil
	case <-time.After(q.config.ShutdownTimeout):
		q.addEvent("ERROR", "Shutdown timeout")
		return fmt.Errorf("shutdown timeout after %v", q.config.ShutdownTimeout)
	}
}

// startWorkers initializes and starts the worker pool
func (q *Queue) startWorkers() {
	for i := 0; i < q.config.WorkerCount; i++ {
		w := &worker{
			id:           i,
			queue:        q,
			stop:         make(chan struct{}),
			wg:           &q.wg,
			status:       "STARTING",
			lastActivity: time.Now(),
		}
		q.workers = append(q.workers, w)
		q.wg.Add(1)
		go w.start()
	}

	q.addEvent("WORKERS", fmt.Sprintf("Started %d workers", q.config.WorkerCount))
}

// startDashboard starts the dashboard rendering loop
func (q *Queue) startDashboard() {
	ticker := time.NewTicker(q.config.DashboardRefreshRate)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			q.dashboard.Render()
		case <-q.ctx.Done():
			return
		}
	}
}

// addEvent adds an event to the recent events log
func (q *Queue) addEvent(eventType, message string) {
	q.stats.AddEvent(eventType, message)
}

// generateID creates a unique message ID
func generateID() string {
	return fmt.Sprintf("msg_%d", time.Now().UnixNano())
}

// SetMetadata sets metadata on a message
func (m *Message) SetMetadata(key string, value any) {
	if m.metadata == nil {
		m.metadata = make(map[string]any)
	}
	m.metadata[key] = value
}

// GetMetadata gets metadata from a message
func (m *Message) GetMetadata(key string) (any, bool) {
	if m.metadata == nil {
		return nil, false
	}
	val, ok := m.metadata[key]
	return val, ok
}
