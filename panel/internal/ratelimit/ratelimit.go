package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

type visitor struct {
	tokens   float64
	lastSeen time.Time
}

type Limiter struct {
	visitors map[string]*visitor
	mu       sync.Mutex
	rate     float64 // tokens per second
	burst    int
}

func New(rate float64, burst int) *Limiter {
	l := &Limiter{visitors: make(map[string]*visitor), rate: rate, burst: burst}
	go l.cleanup()
	return l
}

func (l *Limiter) cleanup() {
	for {
		time.Sleep(time.Minute)
		l.mu.Lock()
		for ip, v := range l.visitors {
			if time.Since(v.lastSeen) > 3*time.Minute {
				delete(l.visitors, ip)
			}
		}
		l.mu.Unlock()
	}
}

func (l *Limiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	v, exists := l.visitors[ip]
	if !exists {
		l.visitors[ip] = &visitor{tokens: float64(l.burst) - 1, lastSeen: time.Now()}
		return true
	}
	elapsed := time.Since(v.lastSeen).Seconds()
	v.tokens += elapsed * l.rate
	if v.tokens > float64(l.burst) {
		v.tokens = float64(l.burst)
	}
	v.lastSeen = time.Now()
	if v.tokens < 1 {
		return false
	}
	v.tokens--
	return true
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		if fwd := r.Header.Get("X-Real-IP"); fwd != "" {
			ip = fwd
		}
		if !l.Allow(ip) {
			http.Error(w, `{"ok":false,"error":"rate_limit_exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
