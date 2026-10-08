package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sablelight/ratelimit-gateway/internal/config"
	"github.com/sablelight/ratelimit-gateway/internal/gateway"
	"github.com/sablelight/ratelimit-gateway/internal/ratelimit"
)

func main() {
	upstream := config.RegisterOption("upstream", "Upstream URL to proxy to", "http://localhost:8081")
	redisAddr := config.RegisterOption("redis.addr", "Redis address", "localhost:6379")
	rate := config.RegisterOption("rate", "Requests per second per key", "10")
	burst := config.RegisterOption("burst", "Burst allowance", "20")
	listenAddr := config.RegisterOption("listen", "Listen address", ":8080")
	metricsAddr := config.RegisterOption("metrics.listen", "Metrics listen address", ":9090")

	log.Printf("Starting ratelimit-gateway")
	log.Printf("  upstream: %s", upstream.GetString())
	log.Printf("  redis: %s", redisAddr.GetString())
	log.Printf("  rate: %d req/s, burst: %d", rate.GetInt(), burst.GetInt())
	log.Printf("  listening on %s", listenAddr.GetString())

	limiter := ratelimit.NewRedisLimiter(
		redisAddr.GetString(),
		rate.GetInt(),
		burst.GetInt(),
	)
	defer limiter.Close()

	gw, err := gateway.New(upstream.GetString(), limiter)
	if err != nil {
		log.Fatalf("create gateway: %v", err)
	}

	// Metrics server with simple JSON endpoint
	metricsMux := http.NewServeMux()
	metricsMux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(gateway.GetMetrics()); err != nil {
			log.Printf("metrics encode failed: %v", err)
		}
	})
	metricsMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("ok")); err != nil {
			log.Printf("healthz write failed: %v", err)
		}
	})
	metricsServer := &http.Server{
		Addr:    metricsAddr.GetString(),
		Handler: metricsMux,
	}
	go func() {
		log.Printf("Metrics on %s", metricsAddr.GetString())
		if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
		}
	}()

	mainServer := &http.Server{
		Addr:         listenAddr.GetString(),
		Handler:      gw,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	go func() {
		if err := mainServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	log.Println("Shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := mainServer.Shutdown(ctx); err != nil {
		log.Printf("main server shutdown: %v", err)
	}
	if err := metricsServer.Shutdown(ctx); err != nil {
		log.Printf("metrics server shutdown: %v", err)
	}
	log.Println("Stopped")
}