package server

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/feedme/aws-eks-ipamd-attribution-exporter/internal/attribution"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Reconciler interface {
	Reconcile(ctx context.Context) (attribution.Result, error)
}

type Config struct {
	BuildCommit        string
	BuildVersion       string
	ClusterName        string
	ListenAddress      string
	ReconcileInterval  time.Duration
	StalenessThreshold time.Duration
}

type Server struct {
	cfg        Config
	reconciler Reconciler
	logger     *slog.Logger
	registry   *prometheus.Registry
	metrics    *metrics
	cache      *cache
}

func New(cfg Config, reconciler Reconciler, logger *slog.Logger) *Server {
	registry := prometheus.NewRegistry()
	metrics := newMetrics(cfg, registry)
	return &Server{
		cfg:        cfg,
		reconciler: reconciler,
		logger:     logger,
		registry:   registry,
		metrics:    metrics,
		cache:      &cache{},
	}
}

func (s *Server) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(s.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/inventory.json", s.inventory)

	httpServer := &http.Server{
		Addr:              s.cfg.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go s.loop(ctx)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("shutdown http server", "error", err)
		}
	}()

	s.logger.Info("starting http server", "listen_address", s.cfg.ListenAddress)
	err := httpServer.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) loop(ctx context.Context) {
	s.reconcileOnce(ctx)
	ticker := time.NewTicker(s.cfg.ReconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.reconcileOnce(ctx)
		}
	}
}

func (s *Server) reconcileOnce(ctx context.Context) {
	result, err := s.reconciler.Reconcile(ctx)
	if err != nil {
		s.logger.Error("reconcile failed", "error", err)
		s.metrics.recordFailure(result)
		s.cache.setError(err)
		return
	}

	s.metrics.recordSuccess(result)
	s.cache.setResult(result)
	s.logger.Info("reconcile succeeded", "duration", result.Duration.String(), "inventory_rows", len(result.Inventory))
}

func (s *Server) healthz(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.cache.snapshot()
	if snapshot.lastSuccess.IsZero() {
		http.Error(w, "no successful reconcile", http.StatusServiceUnavailable)
		return
	}
	age := time.Since(snapshot.lastSuccess)
	if age > s.cfg.StalenessThreshold {
		http.Error(w, "last successful reconcile is stale", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (s *Server) inventory(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.cache.snapshot()
	if snapshot.lastSuccess.IsZero() {
		http.Error(w, "no successful reconcile", http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshot.inventory)
}

type cacheSnapshot struct {
	lastSuccess time.Time
	inventory   []attribution.InventoryRow
	lastError   string
}

type cache struct {
	mu          sync.RWMutex
	lastSuccess time.Time
	inventory   []attribution.InventoryRow
	lastError   string
}

func (c *cache) setResult(result attribution.Result) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastSuccess = result.FinishedAt
	c.inventory = append([]attribution.InventoryRow(nil), result.Inventory...)
	c.lastError = ""
}

func (c *cache) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastError = err.Error()
}

func (c *cache) snapshot() cacheSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return cacheSnapshot{
		lastSuccess: c.lastSuccess,
		inventory:   append([]attribution.InventoryRow(nil), c.inventory...),
		lastError:   c.lastError,
	}
}
