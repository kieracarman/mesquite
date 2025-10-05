package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kieracarman/mesquite"
)

func main() {
	// Suppress default log output when using dashboard
	log.SetOutput(os.NewFile(0, os.DevNull))

	// Create queue with dashboard enabled
	config := mesquite.DefaultConfig()
	config.WorkerCount = 8
	config.MaxQueueSize = 1000
	config.BatchSize = 20
	config.EnableDashboard = true
	config.DashboardRefreshRate = 500 * time.Millisecond

	queue := mesquite.New(config)
	defer queue.Shutdown()

	// Register handlers for different topics
	queue.RegisterHandler("user_events", func(ctx context.Context, msg mesquite.Message) error {
		// Process user events
		time.Sleep(10 * time.Millisecond) // Simulate work
		return nil
	})

	queue.RegisterHandler("orders", func(ctx context.Context, msg mesquite.Message) error {
		// Process orders
		if payload, ok := msg.Payload.(map[string]any); ok {
			// Access order data
			_ = payload
		}
		time.Sleep(15 * time.Millisecond) // Simulate work
		return nil
	})

	queue.RegisterHandler("analytics", func(ctx context.Context, msg mesquite.Message) error {
		// Process analytics events
		time.Sleep(15 * time.Millisecond) // Simulate work
		return nil
	})

	queue.RegisterHandler("notifications", func(ctx context.Context, msg mesquite.Message) error {
		// Process notifications
		time.Sleep(20 * time.Millisecond) // Simulate work

		// Simulate occasional failures for retry demonstration
		if time.Now().UnixNano()%20 == 0 {
			return fmt.Errorf("temporary notification failure")
		}
		return nil
	})

	// Subscribe to a topic for real-time processing
	userEventsSub := queue.Subscribe("user_events")
	go func() {
		for msg := range userEventsSub {
			// Do something with subscribed messages
			_ = msg
		}
	}()

	// Start HTTP server
	httpServer := mesquite.NewHTTPServer(queue, ":8080")
	go func() {
		if err := httpServer.Start(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
		}
	}()
	defer httpServer.Shutdown()

	// Simulate background traffic
	go simulateTraffic(queue)

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down gracefully...")
}

func simulateTraffic(queue *mesquite.Queue) {
	topics := []string{"user_events", "system_events", "analytics", "notifications"}

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	messageCount := 0
	for range ticker.C {
		// Generate 1-3 messages per tick
		for i := 0; i < 1+(messageCount%3); i++ {
			messageCount++
			topic := topics[messageCount%len(topics)]
			payload := map[string]any{
				"id":        messageCount,
				"timestamp": time.Now().Unix(),
				"data":      fmt.Sprintf("Background message #%d", messageCount),
			}
			queue.Publish(topic, payload)
		}
	}
}
