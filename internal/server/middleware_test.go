package server

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/stretchr/testify/assert"
	"golang.org/x/time/rate"
)

// rateLimitMiddlewareTest creates a Gin middleware for rate limiting based on IP,
// mirroring the structure of Server.rateLimitMiddleware but accepting dependencies directly.
// This allows testing the middleware logic in isolation.
func rateLimitMiddlewareTest(
	limit rate.Limit,
	burst int,
	limiters map[string]*rate.Limiter,
	limiterMu *sync.Mutex,
	trustedProxyStrings []string,
) gin.HandlerFunc {
	// Pre-parse trusted proxies once for the middleware
	parsedTrustedProxies, err := iputil.ParseCIDRs(trustedProxyStrings)
	if err != nil {
		// Log critical error during setup simulation
		fmt.Printf("[CRITICAL] Test Setup: Failed to parse trusted proxies for rate limiter: %v\n", err)
		// Return a middleware that always denies in case of config error
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error (rate limiter config)"})
		}
	}

	return func(c *gin.Context) {
		ip := iputil.GetClientIP(c.Request, parsedTrustedProxies)

		limiterMu.Lock()
		limiter, exists := limiters[ip]
		if !exists {
			limiter = rate.NewLimiter(limit, burst)
			limiters[ip] = limiter
		}
		limiterMu.Unlock()

		if !limiter.Allow() {
			fmt.Printf("[INFO] Test: Rate limit exceeded for IP: %s\n", ip)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}

		c.Next()
	}
}

func TestRateLimitMiddlewareIsolated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name                string
		rateLimit           float64 // requests per second
		burst               int
		trustedProxies      []string
		ipAddr              string // Direct IP of the client
		xffHeader           string // Value for X-Forwarded-For
		numRequests         int
		expectedStatusFirst int // Expected status for the first request
		expectedStatusLast  int // Expected status for the last request (if numRequests > 1)
		expectedCallsNext   int // How many times the handler should be called
	}{
		{
			name:                "Single request below limit",
			rateLimit:           1.0, // 1 req/sec
			burst:               1,
			ipAddr:              "192.168.1.10:12345",
			numRequests:         1,
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusOK,
			expectedCallsNext:   1,
		},
		{
			name:                "Multiple requests below limit (burst)",
			rateLimit:           1.0, // 1 req/sec
			burst:               2,
			ipAddr:              "192.168.1.11:12345",
			numRequests:         2,
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusOK,
			expectedCallsNext:   2,
		},
		{
			name:                "Exceed limit",
			rateLimit:           1.0, // 1 req/sec
			burst:               1,
			ipAddr:              "192.168.1.12:12345",
			numRequests:         3, // Send 3 requests quickly
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusTooManyRequests, // Second and third should be limited
			expectedCallsNext:   1,                          // Only the first call should pass
		},
		{
			name:      "Different IPs are independent",
			rateLimit: 1.0, // 1 req/sec
			burst:     1,
			ipAddr:    "192.168.1.13:12345", // This IP will send multiple requests
			// We will also send a request from 192.168.1.14 which should pass
			numRequests:         2, // Send 2 requests from .13
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusTooManyRequests, // Second from .13 should fail
			expectedCallsNext:   1,                          // Only first from .13 passes
		},
		{
			name:                "Trusted proxy - XFF used",
			rateLimit:           1.0, // 1 req/sec
			burst:               1,
			trustedProxies:      []string{"10.0.0.1"},     // Proxy IP
			ipAddr:              "10.0.0.1:54321",         // Request comes from proxy
			xffHeader:           "192.168.1.20, 10.0.0.1", // Real client IP first
			numRequests:         3,
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusTooManyRequests, // Limit based on 192.168.1.20
			expectedCallsNext:   1,
		},
		{
			name:                "Untrusted proxy - XFF ignored",
			rateLimit:           1.0, // 1 req/sec
			burst:               1,
			trustedProxies:      []string{"10.0.0.2"}, // Different proxy IP
			ipAddr:              "10.0.0.1:54321",     // Request comes from untrusted IP
			xffHeader:           "192.168.1.21, 10.0.0.1",
			numRequests:         3,
			expectedStatusFirst: http.StatusOK,
			expectedStatusLast:  http.StatusTooManyRequests, // Limit based on 10.0.0.1
			expectedCallsNext:   1,
		},
		{
			name:                "Invalid trusted proxy config",
			rateLimit:           1.0,
			burst:               1,
			trustedProxies:      []string{"not-a-cidr"}, // Invalid entry
			ipAddr:              "192.168.1.10:12345",
			numRequests:         1,
			expectedStatusFirst: http.StatusInternalServerError, // Middleware creation should fail
			expectedStatusLast:  http.StatusInternalServerError,
			expectedCallsNext:   0, // Handler should not be called
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limiters := make(map[string]*rate.Limiter)
			var limiterMu sync.Mutex

			middleware := rateLimitMiddlewareTest(
				rate.Limit(tt.rateLimit),
				tt.burst,
				limiters,
				&limiterMu,
				tt.trustedProxies,
			)

			// Setup Gin router and handler
			router := gin.New()
			hcallsNextCount := 0
			mockHandler := func(c *gin.Context) {
				hcallsNextCount++
				c.Status(http.StatusOK)
			}
			router.POST("/log", middleware, mockHandler)

			var firstStatus, lastStatus int

			// Send requests
			for i := 0; i < tt.numRequests; i++ {
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("POST", "/log", nil)
				req.RemoteAddr = tt.ipAddr
				if tt.xffHeader != "" {
					req.Header.Set("X-Forwarded-For", tt.xffHeader)
				}
				router.ServeHTTP(w, req)

				if i == 0 {
					firstStatus = w.Code
				}
				lastStatus = w.Code
				// Optional: time.Sleep(time.Millisecond * 10) // Short delay between quick requests
			}

			// Assertions
			assert.Equal(t, tt.expectedStatusFirst, firstStatus, "First request status mismatch")
			assert.Equal(t, tt.expectedStatusLast, lastStatus, "Last request status mismatch")

			// Special case check for independent IPs
			if tt.name == "Different IPs are independent" {
				// Send one request from a different IP
				w := httptest.NewRecorder()
				req, _ := http.NewRequest("POST", "/log", nil)
				req.RemoteAddr = "192.168.1.14:12345" // Different IP
				router.ServeHTTP(w, req)

				assert.Equal(t, http.StatusOK, w.Code, "Request from different IP should be OK")
				assert.Equal(t, tt.expectedCallsNext+1, hcallsNextCount, "Handler call count mismatch for independent IPs")
			} else {
				assert.Equal(t, tt.expectedCallsNext, hcallsNextCount, "Handler call count mismatch")
			}
		})
	}
}
