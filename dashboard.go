package mesquite

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Dashboard handles the visual display
type Dashboard struct {
	queue     *Queue
	mu        sync.Mutex
	maxEvents int
}

// newDashboard creates a new dashboard
func newDashboard(queue *Queue) *Dashboard {
	return &Dashboard{
		queue:     queue,
		maxEvents: 10,
	}
}

// Render renders the dashboard
func (d *Dashboard) Render() {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Clear screen and move cursor to top
	fmt.Print("\033[2J\033[H")

	snapshot := d.queue.stats.Snapshot()

	// Header
	fmt.Println("╔════════════════════════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    MESQUITE - MESSAGE QUEUE DASHBOARD                          ║")
	fmt.Println("╚════════════════════════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// System Overview
	fmt.Println("┌─ SYSTEM OVERVIEW ──────────────────────────────────────────────────────────────┐")
	fmt.Printf("│ Uptime: %-70s │\n", formatDuration(snapshot.Uptime))
	fmt.Printf("│ Workers: %-69d │\n", d.queue.config.WorkerCount)
	fmt.Printf("│ Max Queue Size: %-62d │\n", d.queue.config.MaxQueueSize)
	fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Message Statistics
	fmt.Println("┌─ MESSAGE STATISTICS ───────────────────────────────────────────────────────────┐")
	fmt.Printf("│ Total Queued:    %-61d │\n", snapshot.MessagesQueued)
	fmt.Printf("│ Total Processed: %-61d │\n", snapshot.MessagesProcessed)
	fmt.Printf("│ Total Failed:    %-61d │\n", snapshot.MessagesFailed)
	fmt.Printf("│ Success Rate:    %-58.2f%% │\n", snapshot.SuccessRate)
	fmt.Printf("│ Throughput:      %-54.2f msg/sec │\n", snapshot.Throughput)
	fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Queue Status by Topic
	fmt.Println("┌─ QUEUE STATUS BY TOPIC ────────────────────────────────────────────────────────┐")
	if len(snapshot.QueueSizes) == 0 {
		fmt.Println("│ No active topics                                                               │")
	} else {
		for topic, size := range snapshot.QueueSizes {
			percentage := float64(size) / float64(d.queue.config.MaxQueueSize) * 100
			bar := createProgressBar(int(percentage), 30)
			fmt.Printf("│ %-20s %6d/%-6d [%s] %5.1f%%                  │\n",
				truncate(topic, 20), size, d.queue.config.MaxQueueSize, bar, percentage)
		}
	}
	fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Worker Activity
	fmt.Println("┌─ WORKER ACTIVITY ──────────────────────────────────────────────────────────────┐")
	fmt.Println("│ ID  Status      Processed  Batch  Last Activity                                │")
	fmt.Println("├────────────────────────────────────────────────────────────────────────────────┤")

	for i := 0; i < d.queue.config.WorkerCount; i++ {
		worker := d.queue.workers[i]
		processed := atomic.LoadUint64(&worker.messagesProcessed)
		timeSince := time.Since(worker.lastActivity)

		status := worker.status
		if timeSince > 5*time.Second {
			status = "IDLE"
		}

		fmt.Printf("│ %2d  %-10s  %8d   %4d   %-40s │\n",
			worker.id,
			truncate(status, 10),
			processed,
			worker.currentBatchSize,
			truncate(formatDuration(timeSince)+" ago", 40))
	}
	fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	// Recent Events
	fmt.Println("┌─ RECENT EVENTS ────────────────────────────────────────────────────────────────┐")
	eventCount := len(snapshot.RecentEvents)
	startIdx := 0
	if eventCount > d.maxEvents {
		startIdx = eventCount - d.maxEvents
	}

	if eventCount == 0 {
		fmt.Println("│ No recent events                                                               │")
	} else {
		for i := startIdx; i < eventCount; i++ {
			event := snapshot.RecentEvents[i]
			timeStr := event.Timestamp.Format("15:04:05")
			fmt.Printf("│ [%s] %-10s %-55s │\n",
				timeStr,
				truncate(event.Type, 10),
				truncate(event.Message, 55))
		}
	}
	fmt.Println("└────────────────────────────────────────────────────────────────────────────────┘")
	fmt.Println()

	fmt.Println("Press Ctrl+C to stop")
}

// createProgressBar creates an ASCII progress bar
func createProgressBar(percentage, width int) string {
	if percentage > 100 {
		percentage = 100
	}
	if percentage < 0 {
		percentage = 0
	}

	filled := (percentage * width) / 100
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return bar
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	} else if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	} else if d < time.Hour {
		return fmt.Sprintf("%.1fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}

// truncate truncates a string to a maximum length
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
