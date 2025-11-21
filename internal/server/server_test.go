package server

import (
	"context"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"

	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
)

// Helper function to count sync.Map entries
func syncMapLen(m *sync.Map) int {
	count := 0
	m.Range(func(key, value interface{}) bool {
		count++
		return true
	})
	return count
}

// Helper function to create minimal valid config for testing
func createTestConfig() *config.Config {
	cfg := &config.Config{}

	cfg.AppLog.Level = "INFO"
	cfg.AppLog.ShowHealthLogs = false

	cfg.Server.Host = "localhost"
	cfg.Server.Port = 8080
	cfg.Server.Mode = "standalone"
	cfg.Server.Domain = "test.example.com"
	cfg.Server.Protocol = "http"
	cfg.Server.TrustedProxies = []string{}
	cfg.Server.HealthAllowedIPs = []string{}
	cfg.Server.ClientIPHeader = ""
	cfg.Server.CORS.Enabled = false
	cfg.Server.RequestLimits.MaxBodySize = 102400
	cfg.Server.RequestLimits.RateLimit = 0 // Disabled for most tests
	cfg.Server.JavaScript.GlobalObjectName = "wlp"
	cfg.Server.UnknownRoute.Code = 404
	cfg.Server.UnknownRoute.CacheControl = "no-cache"

	cfg.Security.Token.Secret = "test_secret_that_is_at_least_32_characters_long_for_hmac"
	cfg.Security.Token.Expiration = "1h"

	cfg.LogDestinations = []config.LogDestination{}
	cfg.LogConfig = []config.LogRule{}

	return cfg
}

func TestNewServer(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Creates server successfully", func(t *testing.T) {
		cfg := createTestConfig()
		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)

		assert.NotNil(t, server)
		assert.NotNil(t, server.router)
		// sync.Map is a value type, doesn't need nil check
		assert.NotNil(t, server.shutdownChan)
		assert.Equal(t, cfg, server.config)
	})

	t.Run("Panics with nil Config", func(t *testing.T) {
		loggerMgr := logger.NewManager()
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        nil,
			LoggerManager: loggerMgr,
			RuleProcessor: nil,
			AppLogger:     appLogger,
		}

		assert.Panics(t, func() {
			NewServer(deps)
		})
	})

	t.Run("Panics with nil LoggerManager", func(t *testing.T) {
		cfg := createTestConfig()
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: nil,
			RuleProcessor: nil,
			AppLogger:     appLogger,
		}

		assert.Panics(t, func() {
			NewServer(deps)
		})
	})

	t.Run("Rate limiting enabled when configured", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60 // 60 req/min = 1 req/sec

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)

		assert.Equal(t, rate.Limit(1.0), server.rateLimit, "Should convert 60/min to 1/sec")
		assert.Equal(t, 60, server.burstLimit)

		// Cleanup
		server.Shutdown(context.Background())
	})
}

func TestRateLimiterEntry(t *testing.T) {
	t.Run("Entry tracks last seen time", func(t *testing.T) {
		now := time.Now()
		entry := &rateLimiterEntry{
			limiter:  rate.NewLimiter(1, 5),
			lastSeen: now,
		}

		assert.NotNil(t, entry.limiter)
		assert.Equal(t, now, entry.lastSeen)
	})

	t.Run("Entry updates last seen", func(t *testing.T) {
		entry := &rateLimiterEntry{
			limiter:  rate.NewLimiter(1, 5),
			lastSeen: time.Now().Add(-1 * time.Hour),
		}

		oldTime := entry.lastSeen
		time.Sleep(10 * time.Millisecond)
		entry.lastSeen = time.Now()

		assert.True(t, entry.lastSeen.After(oldTime))
	})
}

func TestCleanupRateLimiters(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Removes old entries", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)
		defer server.Shutdown(context.Background())

		// Add entries with different ages
		// Removed limiterMu - using sync.Map
		server.limiters.Store("192.168.1.1", &rateLimiterEntry{
			limiter:  rate.NewLimiter(1, 5),
			lastSeen: time.Now(),
		})
		server.limiters.Store("192.168.1.2", &rateLimiterEntry{
			limiter:  rate.NewLimiter(1, 5),
			lastSeen: time.Now().Add(-25 * time.Hour), // Old entry
		})
		server.limiters.Store("192.168.1.3", &rateLimiterEntry{
			limiter:  rate.NewLimiter(1, 5),
			lastSeen: time.Now().Add(-48 * time.Hour), // Very old entry
		})
		// Removed limiterMu

		// Verify entries exist
		// Removed limiterMu - using sync.Map
		initialCount := syncMapLen(&server.limiters)
		// Removed limiterMu
		assert.Equal(t, 3, initialCount)

		// Manually trigger cleanup (simulate ticker)
		now := time.Now()
		server.limiters.Range(func(key, value interface{}) bool {
			ip := key.(string)
			entry := value.(*rateLimiterEntry)
			if now.Sub(entry.lastSeen) > 24*time.Hour {
				server.limiters.Delete(ip)
			}
			return true
		})

		// Verify old entries removed, recent kept
		// Removed limiterMu - using sync.Map
		finalCount := syncMapLen(&server.limiters)
		_, hasRecent := server.limiters.Load("192.168.1.1")
		_, hasOld1 := server.limiters.Load("192.168.1.2")
		_, hasOld2 := server.limiters.Load("192.168.1.3")
		// Removed limiterMu

		assert.Equal(t, 1, finalCount, "Should only have 1 entry left")
		assert.True(t, hasRecent, "Recent entry should remain")
		assert.False(t, hasOld1, "Old entry should be removed")
		assert.False(t, hasOld2, "Very old entry should be removed")
	})

	t.Run("Cleanup goroutine stops on shutdown", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)

		// Give goroutine time to start
		time.Sleep(10 * time.Millisecond)

		// Shutdown should close shutdownChan
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()

		err := server.Shutdown(ctx)
		assert.NoError(t, err)

		// Verify shutdown channel is closed
		select {
		case <-server.shutdownChan:
			// Expected - channel should be closed
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Shutdown channel should be closed")
		}
	})
}

func TestShutdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Graceful shutdown succeeds", func(t *testing.T) {
		cfg := createTestConfig()
		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)

		// Start server in goroutine
		go func() {
			server.Start()
		}()

		// Give server time to start
		time.Sleep(50 * time.Millisecond)

		// Shutdown with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := server.Shutdown(ctx)
		assert.NoError(t, err)
	})

	t.Run("Shutdown closes shutdownChan", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60 // Enable cleanup goroutine

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)

		// Verify channel is open
		select {
		case <-server.shutdownChan:
			t.Fatal("Channel should not be closed yet")
		default:
			// Expected
		}

		// Shutdown
		ctx := context.Background()
		server.Shutdown(ctx)

		// Verify channel is closed
		select {
		case <-server.shutdownChan:
			// Expected
		case <-time.After(100 * time.Millisecond):
			t.Fatal("Shutdown channel should be closed after Shutdown()")
		}
	})

	t.Run("Shutdown handles nil httpServer", func(t *testing.T) {
		cfg := createTestConfig()
		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)
		// Don't call Start(), so httpServer remains nil

		ctx := context.Background()
		err := server.Shutdown(ctx)
		assert.NoError(t, err, "Should handle nil httpServer gracefully")
	})
}

func TestRateLimitMiddleware_WithEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Creates new entry on first request", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)
		defer server.Shutdown(context.Background())

		router := gin.New()
		router.Use(server.rateLimitMiddleware())
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "OK")
		})

		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()

		router.ServeHTTP(w, req)

		assert.Equal(t, 200, w.Code)

		// Check entry was created
		// Removed limiterMu - using sync.Map
		val, exists := server.limiters.Load("1.2.3.4")
		assert.True(t, exists, "Entry should be created")
		assert.NotNil(t, val)

		entry := val.(*rateLimiterEntry)
		assert.NotNil(t, entry.limiter)
	})

	t.Run("Updates lastSeen on subsequent requests", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 6000 // High limit to avoid rate limiting

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)
		defer server.Shutdown(context.Background())

		router := gin.New()
		router.Use(server.rateLimitMiddleware())
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "OK")
		})

		// First request
		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.RemoteAddr = "5.5.5.5:12345"
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)

		val1, _ := server.limiters.Load("5.5.5.5")
		firstSeen := val1.(*rateLimiterEntry).lastSeen

		// Wait a bit
		time.Sleep(10 * time.Millisecond)

		// Second request
		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.RemoteAddr = "5.5.5.5:12345"
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		val2, _ := server.limiters.Load("5.5.5.5")
		secondSeen := val2.(*rateLimiterEntry).lastSeen

		assert.True(t, secondSeen.After(firstSeen), "lastSeen should be updated")
	})

	t.Run("Different IPs have separate entries", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.RequestLimits.RateLimit = 60

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		server := NewServer(deps)
		defer server.Shutdown(context.Background())

		router := gin.New()
		router.Use(server.rateLimitMiddleware())
		router.GET("/test", func(c *gin.Context) {
			c.String(200, "OK")
		})

		// Request from IP 1
		req1 := httptest.NewRequest("GET", "/test", nil)
		req1.RemoteAddr = "10.0.0.1:12345"
		w1 := httptest.NewRecorder()
		router.ServeHTTP(w1, req1)

		// Request from IP 2
		req2 := httptest.NewRequest("GET", "/test", nil)
		req2.RemoteAddr = "10.0.0.2:12345"
		w2 := httptest.NewRecorder()
		router.ServeHTTP(w2, req2)

		// Removed limiterMu - using sync.Map
		count := syncMapLen(&server.limiters)
		_, hasIP1 := server.limiters.Load("10.0.0.1")
		_, hasIP2 := server.limiters.Load("10.0.0.2")
		// Removed limiterMu

		assert.Equal(t, 2, count, "Should have 2 separate entries")
		assert.True(t, hasIP1, "Should have entry for IP 1")
		assert.True(t, hasIP2, "Should have entry for IP 2")
	})
}

// Test server lifecycle
func TestServerLifecycle(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("Complete lifecycle: create, start, shutdown", func(t *testing.T) {
		cfg := createTestConfig()
		cfg.Server.Port = 0 // Use random port

		loggerMgr := logger.NewManager()
		ruleProc, _ := rules.NewRuleProcessor(cfg)
		appLogger := logger.GetAppLogger()

		deps := Dependencies{
			Config:        cfg,
			LoggerManager: loggerMgr,
			RuleProcessor: ruleProc,
			AppLogger:     appLogger,
		}

		// Create server
		server := NewServer(deps)
		require.NotNil(t, server)

		// Start in goroutine (would block otherwise)
		serverStarted := make(chan bool)
		go func() {
			serverStarted <- true
			server.Start()
		}()

		<-serverStarted
		time.Sleep(50 * time.Millisecond) // Give server time to start

		// Shutdown gracefully
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		err := server.Shutdown(ctx)
		assert.NoError(t, err)
	})
}

// Benchmark tests
func BenchmarkRateLimitMiddleware_NewEntry(b *testing.B) {
	gin.SetMode(gin.TestMode)
	cfg := createTestConfig()
	cfg.Server.RequestLimits.RateLimit = 6000

	loggerMgr := logger.NewManager()
	ruleProc, _ := rules.NewRuleProcessor(cfg)
	appLogger := logger.GetAppLogger()

	deps := Dependencies{
		Config:        cfg,
		LoggerManager: loggerMgr,
		RuleProcessor: ruleProc,
		AppLogger:     appLogger,
	}

	server := NewServer(deps)
	defer server.Shutdown(context.Background())

	router := gin.New()
	router.Use(server.rateLimitMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "OK")
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkRateLimitMiddleware_ExistingEntry(b *testing.B) {
	gin.SetMode(gin.TestMode)
	cfg := createTestConfig()
	cfg.Server.RequestLimits.RateLimit = 60000 // High limit

	loggerMgr := logger.NewManager()
	ruleProc, _ := rules.NewRuleProcessor(cfg)
	appLogger := logger.GetAppLogger()

	deps := Dependencies{
		Config:        cfg,
		LoggerManager: loggerMgr,
		RuleProcessor: ruleProc,
		AppLogger:     appLogger,
	}

	server := NewServer(deps)
	defer server.Shutdown(context.Background())

	router := gin.New()
	router.Use(server.rateLimitMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.String(200, "OK")
	})

	// Pre-create entry
	req := httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "1.2.3.4:12345"
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		req.RemoteAddr = "1.2.3.4:12345"
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
