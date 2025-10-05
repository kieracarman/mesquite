package mesquite

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// worker processes messages from the queue
type worker struct {
	id                int
	queue             *Queue
	stop              chan struct{}
	wg                *sync.WaitGroup
	messagesProcessed uint64
	currentBatchSize  int
	status            string
	lastActivity      time.Time
}

// start begins the worker's message processing loop
func (w *worker) start() {
	defer w.wg.Done()

	w.status = "IDLE"
	w.lastActivity = time.Now()

	batch := make([]Message, 0, w.queue.config.BatchSize)
	batchTimer := time.NewTimer(w.queue.config.BatchTimeout)
	defer batchTimer.Stop()

	for {
		select {
		case <-w.stop:
			w.status = "STOPPING"
			if len(batch) > 0 {
				w.processBatch(batch)
			}
			return
		case <-batchTimer.C:
			if len(batch) > 0 {
				w.processBatch(batch)
				batch = batch[:0]
			}
			batchTimer.Reset(w.queue.config.BatchTimeout)
		default:
			msg, ok := w.collectMessage()
			if !ok {
				w.status = "IDLE"
				time.Sleep(10 * time.Millisecond)
				continue
			}

			w.status = "COLLECTING"
			batch = append(batch, msg)
			w.currentBatchSize = len(batch)

			if len(batch) >= w.queue.config.BatchSize {
				w.processBatch(batch)
				batch = batch[:0]
				batchTimer.Reset(w.queue.config.BatchTimeout)
			}
		}
	}
}

// collectMessage tries to collect a single message from any queue
func (w *worker) collectMessage() (Message, bool) {
	w.queue.mu.RLock()
	defer w.queue.mu.RUnlock()

	for topic, queue := range w.queue.queues {
		select {
		case msg := <-queue:
			w.queue.stats.mu.Lock()
			w.queue.stats.QueueSize[topic]--
			w.queue.stats.mu.Unlock()

			w.lastActivity = time.Now()
			return msg, true
		default:
			continue
		}
	}
	return Message{}, false
}

// processBatch processes a batch of messages
func (w *worker) processBatch(batch []Message) {
	w.status = "PROCESSING"
	w.currentBatchSize = len(batch)
	w.lastActivity = time.Now()

	for _, msg := range batch {
		w.processMessage(msg)
		atomic.AddUint64(&w.messagesProcessed, 1)
	}

	w.currentBatchSize = 0
	w.status = "IDLE"
}

// processMessage processes a single message
func (w *worker) processMessage(msg Message) {
	ctx, cancel := context.WithTimeout(w.queue.ctx, 30*time.Second)
	defer cancel()

	w.queue.mu.RLock()
	handler, hasHandler := w.queue.handlers[msg.Topic]
	w.queue.mu.RUnlock()

	var err error
	if hasHandler {
		err = handler(ctx, msg)
	} else {
		// Default processing - just simulate some work
		time.Sleep(time.Duration(5+(time.Now().UnixNano()%10)) * time.Millisecond)
	}

	if err != nil && msg.Attempts < w.queue.config.MaxRetries-1 {
		msg.Attempts++
		w.queue.addEvent("RETRY", truncate(msg.ID, 15))

		// Re-queue the message for retry
		go func() {
			time.Sleep(w.queue.config.RetryDelay)
			w.queue.Publish(msg.Topic, msg.Payload)
		}()
		return
	}

	if err != nil {
		atomic.AddUint64(&w.queue.stats.MessagesFailed, 1)
		w.queue.addEvent("FAILED", truncate(msg.ID, 15))
	} else {
		atomic.AddUint64(&w.queue.stats.MessagesProcessed, 1)
	}
}
