package auth

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// RateLimiter 速率限制器
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // 允许的请求数
	window   time.Duration // 时间窗口
}

type visitor struct {
	count     int
	firstSeen time.Time
}

// NewRateLimiter 创建新的速率限制器
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	limiter := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}

	// 启动后台清理 goroutine
	go limiter.cleanupStaleVisitors()

	return limiter
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{
			count:     1,
			firstSeen: time.Now(),
		}
		return true
	}

	// 如果超过时间窗口，重置计数
	if time.Since(v.firstSeen) > rl.window {
		v.count = 1
		v.firstSeen = time.Now()
		return true
	}

	// 检查是否超过限制
	if v.count >= rl.rate {
		return false
	}

	v.count++
	return true
}

// cleanupStaleVisitors 清理过期的访问者记录
func (rl *RateLimiter) cleanupStaleVisitors() {
	for {
		time.Sleep(time.Minute)
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.firstSeen) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// RateLimitMiddleware 速率限制中间件
func RateLimitMiddleware(limiter *RateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := c.ClientIP()
		if !limiter.Allow(ip) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "请求过于频繁，请稍后再试",
			})
			return
		}
		c.Next()
	}
}
