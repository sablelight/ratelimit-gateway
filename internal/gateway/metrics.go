package gateway

import (
	"sync/atomic"
	"time"
)

var (
	requestsTotal   int64
	rateLimitHits   int64
	upstreamLatency int64
)

func IncRequests(path, method string, status int) {
	atomic.AddInt64(&requestsTotal, 1)
}

func IncRateLimitHits() {
	atomic.AddInt64(&rateLimitHits, 1)
}

func ObserveLatency(d time.Duration) {
	atomic.AddInt64(&upstreamLatency, int64(d))
}

func GetMetrics() map[string]any {
	return map[string]any{
		"requests_total":     atomic.LoadInt64(&requestsTotal),
		"rate_limit_hits":    atomic.LoadInt64(&rateLimitHits),
		"upstream_latency_ns": atomic.LoadInt64(&upstreamLatency),
	}
}