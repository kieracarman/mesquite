package mesquite

import (
	"sync"
	"sync/atomic"
	"time"
)

// Stats tracks queue performance metrics
type Stats struct {
	MessagesProcessed uint64
	MessagesFailed    uint64
	MessagesQueued    uint64
	QueueSize         map[string]int64
	mu                sync.RWMutex
	WorkerActivity    []WorkerStats
	RecentEvents      []Event
	eventsMu          sync.Mutex
	StartTime         time.Time
}

// WorkerStats tracks individual worker activity
type WorkerStats struct {
	ID                int
	MessagesProcessed uint64
	CurrentBatchSize  int
	Status            string
	LastActivity      time.Time
}

// Event represents a notable event in the system
type Event struct {
	Timestamp time.Time
	Type      string
	Message   string
}

// StatsSnapshot is a point-in-time snapshot of queue statistics
type StatsSnapshot struct {
	MessagesProcessed uint64
	MessagesFailed    uint64
	MessagesQueued    uint64
	QueueSizes        map[string]int64
	Uptime            time.Duration
	Throughput        float64
	SuccessRate       float64
	WorkerStats       []WorkerStats
	RecentEvents      []Event
}

// newStats creates a new stats tracker
func newStats(workerCount int) *Stats {
	return &Stats{
		QueueSize:      make(map[string]int64),
		WorkerActivity: make([]WorkerStats, workerCount),
		RecentEvents:   make([]Event, 0, 20),
		StartTime:      time.Now(),
	}
}

// AddEvent adds an event to the recent events log
func (s *Stats) AddEvent(eventType, message string) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	event := Event{
		Timestamp: time.Now(),
		Type:      eventType,
		Message:   message,
	}

	s.RecentEvents = append(s.RecentEvents, event)
	if len(s.RecentEvents) > 20 {
		s.RecentEvents = s.RecentEvents[1:]
	}
}

// Snapshot returns a point-in-time snapshot of statistics
func (s *Stats) Snapshot() StatsSnapshot {
	processed := atomic.LoadUint64(&s.MessagesProcessed)
	failed := atomic.LoadUint64(&s.MessagesFailed)
	queued := atomic.LoadUint64(&s.MessagesQueued)
	uptime := time.Since(s.StartTime)

	successRate := float64(0)
	if processed+failed > 0 {
		successRate = float64(processed) / float64(processed+failed) * 100
	}

	throughput := float64(0)
	if uptime.Seconds() > 0 {
		throughput = float64(processed) / uptime.Seconds()
	}

	s.mu.RLock()
	queueSizes := make(map[string]int64)
	for topic, size := range s.QueueSize {
		queueSizes[topic] = size
	}
	s.mu.RUnlock()

	s.eventsMu.Lock()
	events := make([]Event, len(s.RecentEvents))
	copy(events, s.RecentEvents)
	s.eventsMu.Unlock()

	return StatsSnapshot{
		MessagesProcessed: processed,
		MessagesFailed:    failed,
		MessagesQueued:    queued,
		QueueSizes:        queueSizes,
		Uptime:            uptime,
		Throughput:        throughput,
		SuccessRate:       successRate,
		RecentEvents:      events,
	}
}
