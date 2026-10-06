package ratelimit

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter implements a token-bucket rate limiter backed by Redis.
type Limiter struct {
	client *redis.Client
	rate   int       // tokens added per second
	burst  int       // max bucket size
}

// Rate returns the configured requests per second.
func (l *Limiter) Rate() int {
	return l.rate
}

// NewRedisLimiter creates a limiter with the given rate (req/s) and burst.
func NewRedisLimiter(addr string, rate, burst int) *Limiter {
	return &Limiter{
		client: redis.NewClient(&redis.Options{Addr: addr}),
		rate:   rate,
		burst:  burst,
	}
}

// Allow checks if a request for key is allowed.
// Returns (allowed, retryAfter, err).
func (l *Limiter) Allow(ctx context.Context, key string) (bool, time.Duration, error) {
	now := time.Now().UnixMilli()
	redisKey := "rl:" + key

	// Token bucket algorithm in Lua:
	// - bucket stores: tokens (float), last_refill_ms (int)
	// - on each request: refill based on elapsed time, then try to take 1 token
	script := redis.NewScript(`
		local key = KEYS[1]
		local rate = tonumber(ARGV[1])
		local burst = tonumber(ARGV[2])
		local now = tonumber(ARGV[3])
		local cost = tonumber(ARGV[4])

		local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
		local tokens = tonumber(bucket[1])
		local last_refill = tonumber(bucket[2])

		if tokens == nil then
			tokens = burst
			last_refill = now
		end

		-- refill
		local elapsed = now - last_refill
		local refill = (elapsed / 1000.0) * rate
		tokens = math.min(burst, tokens + refill)

		local allowed = false
		local retry_after = 0
		if tokens >= cost then
			tokens = tokens - cost
			allowed = true
		else
			-- time until 1 token available (ms)
			retry_after = math.ceil((cost - tokens) / rate * 1000)
		end

		redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
		redis.call('EXPIRE', key, math.ceil(burst / rate) + 10)

		return {allowed, tokens, retry_after}
	`)

	cost := 1
	res, err := script.Run(ctx, l.client, []string{redisKey},
		l.rate, l.burst, now, cost).Slice()
	if err != nil {
		return false, 0, err
	}

	allowed, _ := strconv.ParseBool(res[0].(string))
	retryAfterMs, _ := strconv.Atoi(res[2].(string))

	if !allowed {
		return false, time.Duration(retryAfterMs) * time.Millisecond, nil
	}
	return true, 0, nil
}

// Close closes the Redis client.
func (l *Limiter) Close() error {
	return l.client.Close()
}