# Mesquite 🌵

A high-performance, observable message queue library for Go with real-time monitoring.

## Features

- 🚀 High-performance concurrent message processing
- 📊 Real-time monitoring dashboard
- 🔄 Automatic retry with configurable policies
- 📦 Batch processing for efficiency
- 🎯 Topic-based routing
- 👂 Pub/Sub support
- 🛡️ Graceful shutdown
- 📈 Built-in statistics and observability

## Installation

```bash
go get github.com/yourusername/mesquite
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/yourusername/mesquite"
)

func main() {
    // Create queue with defaults
    queue := mesquite.NewWithDefaults()
    defer queue.Shutdown()

    // Register a handler
    queue.RegisterHandler("orders", func(ctx context.Context, msg mesquite.Message) error {
        fmt.Printf("Processing order: %v\n", msg.Payload)
        return nil
    })

    // Publish messages
    queue.Publish("orders", map[string]interface{}{
        "order_id": "12345",
        "amount":   99.99,
    })
}
```

## With Dashboard

```go
config := mesquite.DefaultConfig()
config.EnableDashboard = true
config.WorkerCount = 10

queue := mesquite.New(config)
defer queue.Shutdown()

// Your application code...
```

## Configuration

```go
config := mesquite.Config{
    MaxQueueSize:         10000,              // Max messages per topic
    WorkerCount:          10,                 // Number of workers
    MaxRetries:           3,                  // Retry attempts
    RetryDelay:           time.Second,        // Delay between retries
    BatchSize:            100,                // Messages per batch
    BatchTimeout:         100 * time.Millisecond,
    ShutdownTimeout:      30 * time.Second,
    EnableDashboard:      true,               // Enable real-time dashboard
    DashboardRefreshRate: 500 * time.Millisecond,
}

queue := mesquite.New(config)
```

## Advanced Usage

### Subscribe to Topics

```go
sub := queue.Subscribe("events")
go func() {
    for msg := range sub {
        fmt.Printf("Received: %v\n", msg.Payload)
    }
}()
```

### Get Statistics

```go
stats := queue.GetStats()
fmt.Printf("Processed: %d\n", stats.MessagesProcessed)
fmt.Printf("Throughput: %.2f msg/sec\n", stats.Throughput)
```

### Error Handling and Retries

```go
queue.RegisterHandler("critical", func(ctx context.Context, msg mesquite.Message) error {
    if err := processMessage(msg); err != nil {
        return err // Will trigger retry
    }
    return nil
})
```

## License

MIT
