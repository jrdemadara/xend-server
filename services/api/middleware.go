package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"xend.chat/m/pkg/httputil"
)

type authRateLimiter struct {
	redis  *goredis.Client
	limit  int64
	window time.Duration
}

func newAuthRateLimiter(redisClient *goredis.Client, limit int64, window time.Duration) *authRateLimiter {
	return &authRateLimiter{redis: redisClient, limit: limit, window: window}
}

func (rl *authRateLimiter) middlewareHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := requestIP(r)
		key := "rl:auth:" + ip + ":" + r.Method

		count, err := rl.redis.Incr(r.Context(), key).Result()
		if err != nil {
			httputil.Error(w, http.StatusInternalServerError, "internal_error", "rate limiter unavailable")
			return
		}
		if count == 1 {
			if err := rl.redis.Expire(r.Context(), key, rl.window).Err(); err != nil {
				httputil.Error(w, http.StatusInternalServerError, "internal_error", "rate limiter unavailable")
				return
			}
		}

		ttl, _ := rl.redis.TTL(r.Context(), key).Result()
		remaining := rl.limit - count
		if remaining < 0 {
			remaining = 0
		}
		w.Header().Set("X-RateLimit-Limit", strconv.FormatInt(rl.limit, 10))
		w.Header().Set("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))
		if ttl > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(int(ttl.Seconds())))
		}

		if count > rl.limit {
			httputil.Error(w, http.StatusTooManyRequests, "rate_limited", "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requestIP(r *http.Request) string {
	xff := strings.TrimSpace(r.Header.Get("X-Forwarded-For"))
	if xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	ip, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return ip
}
