package gateway

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/sablelight/ratelimit-gateway/internal/ratelimit"
)

type Gateway struct {
	upstream *url.URL
	limiter  *ratelimit.Limiter
	proxy    *httputil.ReverseProxy
}

func New(upstreamURL string, limiter *ratelimit.Limiter) (*Gateway, error) {
	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, err
	}

	director := func(req *http.Request) {
		req.Header.Set("X-Forwarded-Host", req.Host)
		req.Header.Set("X-Forwarded-Proto", "http")
		req.Host = target.Host
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)
	}

	// Need to type assert DefaultTransport to *http.Transport
	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		baseTransport = &http.Transport{}
	}

	g := &Gateway{
		upstream: target,
		limiter:  limiter,
	}

	g.proxy = &httputil.ReverseProxy{Director: director}
	g.proxy.Transport = &rateLimitedTransport{
		Transport: baseTransport,
		gateway:   g,
	}

	return g, nil
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	g.proxy.ServeHTTP(w, r)
	ObserveLatency(time.Since(start))
}

type rateLimitedTransport struct {
	*http.Transport
	gateway *Gateway
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	apiKey := extractAPIKey(req)
	if apiKey == "" {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       http.NoBody,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
		}, nil
	}

	allowed, retryAfter, err := t.gateway.limiter.Allow(req.Context(), apiKey)
	if err != nil {
		log.Printf("rate limiter error: %v", err)
		return &http.Response{
			StatusCode: http.StatusInternalServerError,
			Body:       http.NoBody,
		}, nil
	}

	if !allowed {
		IncRateLimitHits()
		resp := &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Body:       http.NoBody,
			Header: http.Header{
				"Content-Type":        []string{"application/json"},
				"Retry-After":         []string{strconv.FormatInt(int64(retryAfter.Seconds()), 10)},
				"X-RateLimit-Limit":   []string{strconv.Itoa(t.gateway.limiter.Rate())},
				"X-RateLimit-Remaining": []string{"0"},
			},
		}
		IncRequests(req.URL.Path, req.Method, 429)
		return resp, nil
	}

	IncRequests(req.URL.Path, req.Method, 200)
	return t.Transport.RoundTrip(req)
}

func extractAPIKey(r *http.Request) string {
	if key := r.Header.Get("X-API-Key"); key != "" {
		return key
	}
	if key := r.URL.Query().Get("api_key"); key != "" {
		return key
	}
	return ""
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}