package ratelimit

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLimiter_Allow(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx := context.Background()
	client.FlushDB(ctx)
	defer client.Close()

	limiter := NewRedisLimiter("localhost:6379", 5, 10) // 5 req/s, burst 10
	defer limiter.Close()

	key := "test-key"

	// First 10 requests should be allowed (burst)
	for i := 0; i < 10; i++ {
		allowed, _, err := limiter.Allow(ctx, key)
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if !allowed {
			t.Errorf("request %d: expected allowed, got denied", i)
		}
	}

	// 11th should be denied
	allowed, retryAfter, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Error("expected 11th request to be denied")
	}
	if retryAfter <= 0 {
		t.Errorf("expected positive retryAfter, got %v", retryAfter)
	}
}

func TestRedisLimiter_Refill(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx := context.Background()
	client.FlushDB(ctx)
	defer client.Close()

	limiter := NewRedisLimiter("localhost:6379", 10, 10) // 10 req/s, burst 10
	defer limiter.Close()

	key := "refill-test"

	// Exhaust burst
	for i := 0; i < 10; i++ {
		allowed, _, _ := limiter.Allow(ctx, key)
		if !allowed {
			t.Fatalf("request %d should be allowed", i)
		}
	}

	// Wait for refill (1 second = 10 tokens)
	time.Sleep(1100 * time.Millisecond)

	// Should be allowed again
	allowed, _, err := limiter.Allow(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Error("expected request to be allowed after refill")
	}
}

func TestRedisLimiter_DifferentKeysIndependent(t *testing.T) {
	client := redis.NewClient(&redis.Options{Addr: "localhost:6379"})
	ctx := context.Background()
	client.FlushDB(ctx)
	defer client.Close()

	limiter := NewRedisLimiter("localhost:6379", 2, 2)
	defer limiter.Close()

	ctx = context.Background()

	// Exhaust key1
	for i := 0; i < 2; i++ {
		allowed, _, _ := limiter.Allow(ctx, "key1")
		if !allowed {
			t.Fatalf("key1 request %d should be allowed", i)
		}
	}
	allowed, _, _ := limiter.Allow(ctx, "key1")
	if allowed {
		t.Error("key1 should be exhausted")
	}

	// key2 should be unaffected
	allowed, _, _ = limiter.Allow(ctx, "key2")
	if !allowed {
		t.Error("key2 should be allowed")
	}
	allowed, _, _ = limiter.Allow(ctx, "key2")
	if !allowed {
		t.Error("key2 second request should be allowed")
	}
	allowed, _, _ = limiter.Allow(ctx, "key2")
	if allowed {
		t.Error("key2 should be exhausted")
	}
}