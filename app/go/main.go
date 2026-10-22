// txn-api: a tiny fake transactions service used for the platform exercise.
package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "HTTP requests by method, route and status code.",
	}, []string{"method", "route", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and route.",
		Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5},
	}, []string{"method", "route"})

	txnProcessed = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "txn_processed_total",
		Help: "Transactions processed by currency and result (ok, rejected, error).",
	}, []string{"currency", "result"})
)

var currencies = map[string]bool{"USD": true, "EUR": true, "GBP": true, "USDC": true}

type config struct {
	port     string
	region   string
	failRate float64
	logLevel slog.Level
}

type transaction struct {
	ID          string    `json:"id"`
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Region      string    `json:"region"`
	CreatedAt   time.Time `json:"created_at"`
}

type server struct {
	cfg   config
	log   *slog.Logger
	ready atomic.Bool

	mu    sync.RWMutex
	store map[string]transaction
}

func loadConfig() (config, []string) {
	var missing []string
	cfg := config{port: "8000", logLevel: slog.LevelInfo}

	if v := os.Getenv("PORT"); v != "" {
		cfg.port = v
	}
	cfg.region = os.Getenv("TXN_REGION")
	if cfg.region == "" {
		missing = append(missing, "TXN_REGION")
	}
	if v := os.Getenv("TXN_FAIL_RATE"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.failRate = f
		}
	}
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		cfg.logLevel = slog.LevelDebug
	case "warn":
		cfg.logLevel = slog.LevelWarn
	case "error":
		cfg.logLevel = slog.LevelError
	}
	return cfg, missing
}

func main() {
	cfg, missing := loadConfig()
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.logLevel}))

	if len(missing) > 0 {
		log.Error("missing required environment variable", "vars", strings.Join(missing, ","))
		os.Exit(1)
	}

	s := &server{cfg: cfg, log: log, store: map[string]transaction{}}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("POST /api/transactions", s.instrument("/api/transactions", s.handleCreate))
	mux.HandleFunc("GET /api/transactions/{id}", s.instrument("/api/transactions/{id}", s.handleGet))

	s.ready.Store(true)
	log.Info("starting txn-api", "port", cfg.port, "region", cfg.region, "fail_rate", cfg.failRate)
	if err := http.ListenAndServe(":"+cfg.port, mux); err != nil {
		log.Error("server exited", "error", err.Error())
		os.Exit(1)
	}
}

func (s *server) handleReady(w http.ResponseWriter, r *http.Request) {
	if !s.ready.Load() {
		http.Error(w, "not ready\n", http.StatusServiceUnavailable)
		return
	}
	w.Write([]byte("ready\n"))
}

// statusRecorder captures the status code and error message a handler wrote.
type statusRecorder struct {
	http.ResponseWriter
	status int
	errMsg string
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *server) instrument(route string, h func(*statusRecorder, *http.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		h(rec, r)
		elapsed := time.Since(start)

		status := strconv.Itoa(rec.status)
		httpRequests.WithLabelValues(r.Method, route, status).Inc()
		httpDuration.WithLabelValues(r.Method, route).Observe(elapsed.Seconds())

		attrs := []any{
			"method", r.Method,
			"route", route,
			"status", rec.status,
			"duration_ms", float64(elapsed.Microseconds()) / 1000,
			"region", s.cfg.region,
		}
		switch {
		case rec.status >= 500:
			s.log.Error("request failed", append(attrs, "error", rec.errMsg)...)
		case rec.status >= 400:
			s.log.Warn("request rejected", append(attrs, "error", rec.errMsg)...)
		default:
			s.log.Info("request handled", attrs...)
		}
	}
}

func writeJSON(w *statusRecorder, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w *statusRecorder, status int, msg string) {
	w.errMsg = msg
	writeJSON(w, status, map[string]string{"error": msg})
}

// simulateWork sleeps for a plausible amount of time, with a slow tail.
func simulateWork() {
	ms := 5 + rand.IntN(40)
	if rand.Float64() < 0.05 {
		ms += 200 + rand.IntN(300)
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func newID() string {
	b := make([]byte, 8)
	_, _ = crand.Read(b)
	return "txn_" + hex.EncodeToString(b)
}

func (s *server) handleCreate(w *statusRecorder, r *http.Request) {
	var req struct {
		AmountCents int64  `json:"amount_cents"`
		Currency    string `json:"currency"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		txnProcessed.WithLabelValues("unknown", "rejected").Inc()
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	cur := strings.ToUpper(req.Currency)
	if !currencies[cur] {
		txnProcessed.WithLabelValues("unknown", "rejected").Inc()
		writeError(w, http.StatusBadRequest, "unsupported currency")
		return
	}
	if req.AmountCents <= 0 {
		txnProcessed.WithLabelValues(cur, "rejected").Inc()
		writeError(w, http.StatusBadRequest, "amount_cents must be positive")
		return
	}

	simulateWork()
	if rand.Float64() < s.cfg.failRate {
		txnProcessed.WithLabelValues(cur, "error").Inc()
		writeError(w, http.StatusInternalServerError, "ledger write timed out")
		return
	}

	t := transaction{ID: newID(), AmountCents: req.AmountCents, Currency: cur, Region: s.cfg.region, CreatedAt: time.Now().UTC()}
	s.mu.Lock()
	s.store[t.ID] = t
	s.mu.Unlock()
	txnProcessed.WithLabelValues(cur, "ok").Inc()
	writeJSON(w, http.StatusCreated, t)
}

func (s *server) handleGet(w *statusRecorder, r *http.Request) {
	simulateWork()
	if rand.Float64() < s.cfg.failRate/2 {
		writeError(w, http.StatusServiceUnavailable, "ledger read replica unavailable")
		return
	}
	s.mu.RLock()
	t, ok := s.store[r.PathValue("id")]
	s.mu.RUnlock()
	if !ok {
		writeError(w, http.StatusNotFound, "transaction not found")
		return
	}
	writeJSON(w, http.StatusOK, t)
}
