package routes

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// rateLimiter — простой in-memory лимитер по ключу (IP) со скользящим окном.
type rateLimiter struct {
	limit  int
	window time.Duration

	mu   sync.Mutex
	hits map[string][]time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	l := &rateLimiter{
		limit:  limit,
		window: window,
		hits:   map[string][]time.Time{},
	}
	go l.cleanup()
	return l
}

// allow возвращает true, если попытка в пределах лимита, и фиксирует её.
func (l *rateLimiter) allow(key string) bool {
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	hits := prunable(l.hits[key], cutoff)
	if len(hits) >= l.limit {
		l.hits[key] = hits
		return false
	}
	l.hits[key] = append(hits, now)
	return true
}

// cleanup периодически выбрасывает протухшие записи.
func (l *rateLimiter) cleanup() {
	interval := l.window
	if interval > time.Minute {
		interval = time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for range ticker.C {
		cutoff := time.Now().Add(-l.window)
		l.mu.Lock()
		for key, hits := range l.hits {
			hits = prunable(hits, cutoff)
			if len(hits) == 0 {
				delete(l.hits, key)
			} else {
				l.hits[key] = hits
			}
		}
		l.mu.Unlock()
	}
}

func prunable(hits []time.Time, cutoff time.Time) []time.Time {
	i := 0
	for i < len(hits) && !hits[i].After(cutoff) {
		i++
	}
	return hits[i:]
}

func (l *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.allow(clientIP(r)) {
			http.Error(w, "Слишком много попыток. Попробуйте позже.", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
