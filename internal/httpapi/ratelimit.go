package httpapi

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

type ipRateLimiter struct {
	mu       sync.Mutex
	visitors map[string]*visitor
	rate     rate.Limit
	burst    int
	now      func() time.Time
}

func newIPRateLimiter(requestsPerMinute, burst int) *ipRateLimiter {
	return &ipRateLimiter{visitors: make(map[string]*visitor), rate: rate.Limit(float64(requestsPerMinute) / 60), burst: burst, now: time.Now}
}

func (l *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr
		}
		l.mu.Lock()
		entry := l.visitors[host]
		if entry == nil {
			entry = &visitor{limiter: rate.NewLimiter(l.rate, l.burst)}
			l.visitors[host] = entry
		}
		entry.lastSeen = l.now()
		allowed := entry.limiter.Allow()
		if len(l.visitors) > 10000 {
			cutoff := l.now().Add(-10 * time.Minute)
			for key, item := range l.visitors {
				if item.lastSeen.Before(cutoff) {
					delete(l.visitors, key)
				}
			}
		}
		l.mu.Unlock()
		if !allowed {
			w.Header().Set("Retry-After", "1")
			writeError(w, http.StatusTooManyRequests, "RATE_LIMITED", r)
			return
		}
		next.ServeHTTP(w, r)
	})
}
