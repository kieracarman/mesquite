package mesquite

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// HTTPServer provides HTTP endpoints for the message queue
type HTTPServer struct {
	queue  *Queue
	server *http.Server
	mux    *http.ServeMux
}

// PublishRequest represents an HTTP publish request
type PublishRequest struct {
	Topic   string `json:"topic"`
	Payload any    `json:"payload"`
}

// PublishResponse represents an HTTP publish response
type PublishResponse struct {
	Status    string `json:"status"`
	MessageID string `json:"message_id,omitempty"`
	Error     string `json:"error,omitempty"`
}

// StatsResponse represents an HTTP stats response
type StatsResponse struct {
	MessagesProcessed uint64           `json:"messages_processed"`
	MessagesFailed    uint64           `json:"messages_failed"`
	MessagesQueued    uint64           `json:"messages_queued"`
	Uptime            string           `json:"uptime"`
	Throughput        float64          `json:"throughput"`
	SuccessRate       float64          `json:"success_rate"`
	QueueSizes        map[string]int64 `json:"queue_sizes"`
	WorkerCount       int              `json:"worker_count"`
}

// HealthResponse represents a health check response
type HealthResponse struct {
	Status  string `json:"status"`
	Uptime  string `json:"uptime"`
	Workers int    `json:"workers"`
}

// NewHTTPServer creates a new HTTP server for the message queue
func NewHTTPServer(queue *Queue, addr string) *HTTPServer {
	mux := http.NewServeMux()

	server := &HTTPServer{
		queue: queue,
		mux:   mux,
		server: &http.Server{
			Addr:         addr,
			Handler:      mux,
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 15 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
	}

	// Register routes
	mux.HandleFunc("/publish", server.handlePublish)
	mux.HandleFunc("/stats", server.handleStats)
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/", server.handleIndex)

	return server
}

// Start starts the HTTP server
func (s *HTTPServer) Start() error {
	s.queue.addEvent("HTTP", fmt.Sprintf("Starting server on %s", s.server.Addr))
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the HTTP server
func (s *HTTPServer) Shutdown() error {
	s.queue.addEvent("HTTP", "Shutting down server")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return s.server.Shutdown(ctx)
}

// handlePublish handles POST /publish requests
func (s *HTTPServer) handlePublish(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req PublishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.respondError(w, http.StatusBadRequest, "Invalid JSON: "+err.Error())
		return
	}

	if req.Topic == "" {
		s.respondError(w, http.StatusBadRequest, "Topic is required")
		return
	}

	// Generate message ID before publishing
	msgID := generateID()

	if err := s.queue.Publish(req.Topic, req.Payload); err != nil {
		s.respondError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	s.queue.addEvent("HTTP", fmt.Sprintf("Published to %s", req.Topic))

	s.respondJSON(w, http.StatusAccepted, PublishResponse{
		Status:    "accepted",
		MessageID: msgID,
	})
}

// handleStats handles GET /stats requests
func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	snapshot := s.queue.GetStats()

	response := StatsResponse{
		MessagesProcessed: snapshot.MessagesProcessed,
		MessagesFailed:    snapshot.MessagesFailed,
		MessagesQueued:    snapshot.MessagesQueued,
		Uptime:            snapshot.Uptime.String(),
		Throughput:        snapshot.Throughput,
		SuccessRate:       snapshot.SuccessRate,
		QueueSizes:        snapshot.QueueSizes,
		WorkerCount:       s.queue.config.WorkerCount,
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleHealth handles GET /health requests
func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.respondError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	snapshot := s.queue.GetStats()

	response := HealthResponse{
		Status:  "healthy",
		Uptime:  snapshot.Uptime.String(),
		Workers: s.queue.config.WorkerCount,
	}

	s.respondJSON(w, http.StatusOK, response)
}

// handleIndex handles GET / requests
func (s *HTTPServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Mesquite Message Queue</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #2c3e50; margin-bottom: 10px; }
        .subtitle { color: #7f8c8d; margin-bottom: 30px; }
        .endpoint { background: #ecf0f1; padding: 15px; margin: 10px 0; border-radius: 4px; }
        .method { display: inline-block; padding: 4px 8px; border-radius: 3px; font-weight: bold; margin-right: 10px; }
        .post { background: #3498db; color: white; }
        .get { background: #2ecc71; color: white; }
        code { background: #34495e; color: #ecf0f1; padding: 2px 6px; border-radius: 3px; font-family: monospace; }
        pre { background: #2c3e50; color: #ecf0f1; padding: 15px; border-radius: 4px; overflow-x: auto; }
        .example { margin-top: 10px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🌵 Mesquite Message Queue</h1>
        <p class="subtitle">High-performance message queue with real-time observability</p>
        
        <h2>API Endpoints</h2>
        
        <div class="endpoint">
            <span class="method post">POST</span>
            <code>/publish</code>
            <p>Publish a message to a topic</p>
            <div class="example">
                <strong>Example:</strong>
                <pre>curl -X POST http://localhost:8080/publish \
  -H "Content-Type: application/json" \
  -d '{"topic":"orders","payload":{"order_id":"12345","amount":99.99}}'</pre>
            </div>
        </div>

        <div class="endpoint">
            <span class="method get">GET</span>
            <code>/stats</code>
            <p>Get queue statistics</p>
            <div class="example">
                <strong>Example:</strong>
                <pre>curl http://localhost:8080/stats</pre>
            </div>
        </div>

        <div class="endpoint">
            <span class="method get">GET</span>
            <code>/health</code>
            <p>Health check endpoint</p>
            <div class="example">
                <strong>Example:</strong>
                <pre>curl http://localhost:8080/health</pre>
            </div>
        </div>

        <h2>Quick Start</h2>
        <pre>// Publish a message
curl -X POST http://localhost:8080/publish \
  -H "Content-Type: application/json" \
  -d '{"topic":"events","payload":{"event":"user_login","user_id":"123"}}'

// Check stats
curl http://localhost:8080/stats | jq

// Health check
curl http://localhost:8080/health</pre>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// respondJSON responds with JSON
func (s *HTTPServer) respondJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// respondError responds with an error
func (s *HTTPServer) respondError(w http.ResponseWriter, status int, message string) {
	s.respondJSON(w, status, PublishResponse{
		Status: "error",
		Error:  message,
	})
}
