package server

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	configparser "github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/handler"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	"golang.org/x/time/rate"
)

// Dependencies holds the dependencies needed by the server.
type Dependencies struct {
	Config        *configparser.Config
	LoggerManager *logger.Manager
	RuleProcessor *rules.RuleProcessor
}

// Server represents the HTTP server
type Server struct {
	router        *gin.Engine
	config        *configparser.Config
	loggerManager *logger.Manager
	ruleProcessor *rules.RuleProcessor
	// Rate limiting specific
	limiters   map[string]*rate.Limiter
	limiterMu  sync.Mutex
	rateLimit  rate.Limit
	burstLimit int
	deps       Dependencies
}

// NewServer creates a new server instance with its dependencies.
func NewServer(deps Dependencies) *Server {
	// Validate dependencies
	if deps.Config == nil {
		panic("server: Config dependency cannot be nil")
	}
	if deps.LoggerManager == nil {
		panic("server: LoggerManager dependency cannot be nil")
	}
	if deps.RuleProcessor == nil {
		panic("server: RuleProcessor dependency cannot be nil")
	}

	fmt.Printf("[DEBUG] Creating server with mode: %s, path_prefix: %s\n", deps.Config.Server.Mode, deps.Config.Server.PathPrefix)

	// Set Gin mode
	if deps.Config.Server.Mode == "production" {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(gin.Logger()) // Use Gin's logger middleware

	if deps.Config.Server.CORS.Enabled {
		router.Use(corsMiddleware(deps.Config.Server.CORS.AllowedOrigins, deps.Config.Server.CORS.MaxAge))
	}

	server := &Server{
		router:        router,
		config:        deps.Config,
		loggerManager: deps.LoggerManager,
		ruleProcessor: deps.RuleProcessor,
		limiters:      make(map[string]*rate.Limiter),
		deps:          deps,
	}

	// Initialize rate limiter settings
	if deps.Config.Server.RequestLimits.RateLimit > 0 {
		// Convert requests per minute to requests per second
		server.rateLimit = rate.Limit(float64(deps.Config.Server.RequestLimits.RateLimit) / 60.0)
		// Set burst limit (e.g., allow bursts up to the per-minute limit)
		server.burstLimit = deps.Config.Server.RequestLimits.RateLimit
		fmt.Printf("[INFO] Rate limiting enabled for /log: Rate=%.2f req/sec, Burst=%d\n", server.rateLimit, server.burstLimit)
	} else {
		server.rateLimit = rate.Inf // No limit
		server.burstLimit = 0
		fmt.Println("[INFO] Rate limiting disabled for /log.")
	}

	// Zde naparsujeme dobu expirace tokenu jednou
	tokenExpirationDur, err := configparser.ParseDuration(deps.Config.Security.Token.Expiration)
	if err != nil {
		// Validation should have caught this, but handle defensively
		panic(fmt.Sprintf("server: failed to parse pre-validated token expiration '%s': %v", deps.Config.Security.Token.Expiration, err))
	}

	server.setupRoutes(tokenExpirationDur)
	return server
}

// setupRoutes configures the HTTP routes
func (s *Server) setupRoutes(tokenExpirationDur time.Duration) {
	// Define base path
	basePath := "/"
	if s.config.Server.Mode == "embedded" && s.config.Server.PathPrefix != "" {
		basePath = s.config.Server.PathPrefix
		if basePath[0] != '/' { // Ensure leading slash
			basePath = "/" + basePath
		}
	}
	// Ensure base path ends with a slash if it's not root
	if basePath != "/" && basePath[len(basePath)-1] != '/' {
		basePath += "/"
	}

	fmt.Printf("[DEBUG] Setting up routes with basePath: %s\n", basePath)

	group := s.router.Group(basePath)
	{
		// Health check endpoint (no rate limit)
		group.GET("health", func(c *gin.Context) {
			fmt.Printf("[DEBUG] Health endpoint called with method %s, path %s\n", c.Request.Method, c.Request.URL.Path)
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		group.HEAD("health", func(c *gin.Context) {
			fmt.Printf("[DEBUG] Health endpoint called with method %s, path %s\n", c.Request.Method, c.Request.URL.Path)
			c.Status(http.StatusOK)
		})

		// Version endpoint (no rate limit)
		group.GET("version", handler.VersionHandler)

		// Logger.js endpoint (no rate limit)
		loggerJSDeps := handler.LoggerJSHandlerDeps{
			RuleProcessor:      s.ruleProcessor,
			Config:             s.config,
			TrustedProxies:     s.config.Server.TrustedProxies,
			TokenExpirationDur: tokenExpirationDur,
		}
		group.GET("logger.js", handler.NewLoggerJSHandler(loggerJSDeps))

		// Log endpoint - Apply rate limiter middleware first
		logGroup := group.Group("/log")
		if s.rateLimit != rate.Inf {
			logGroup.Use(s.rateLimitMiddleware())
		}
		{
			// Log Handler Dependencies
			logDeps := handler.LogHandlerDependencies{
				LoggerManager:  s.deps.LoggerManager,
				TokenSecret:    s.deps.Config.Security.Token.Secret,
				RuleProcessor:  s.deps.RuleProcessor,
				TrustedProxies: s.deps.Config.Server.TrustedProxies,
				Config:         s.deps.Config,
			}

			// Register Log Handler
			logGroup.POST("", handler.NewLogHandler(logDeps)) // POST /log
		}
	}
}

// rateLimitMiddleware creates a Gin middleware for rate limiting based on IP.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	// Pre-parse trusted proxies once for the middleware
	parsedTrustedProxies, err := iputil.ParseCIDRs(s.config.Server.TrustedProxies)
	if err != nil {
		// Log critical error during server setup
		fmt.Printf("[CRITICAL] Failed to parse trusted proxies for rate limiter: %v\n", err)
		// Return a middleware that always denies? Or panic?
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error (rate limiter config)"})
		}
	}

	return func(c *gin.Context) {
		ip := iputil.GetClientIP(c.Request, parsedTrustedProxies)

		s.limiterMu.Lock()
		limiter, exists := s.limiters[ip]
		if !exists {
			limiter = rate.NewLimiter(s.rateLimit, s.burstLimit)
			s.limiters[ip] = limiter
		}
		s.limiterMu.Unlock()

		if !limiter.Allow() {
			// Log the rate limit exceedance internally
			fmt.Printf("[INFO] Rate limit exceeded for IP: %s\n", ip)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}

		c.Next()
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	fmt.Printf("[INFO] Starting server on %s\n", addr)
	// Consider using http.Server for more control over shutdown
	return s.router.Run(addr)
}

// corsMiddleware creates a middleware for CORS
func corsMiddleware(allowedOrigins []string, maxAge int) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		found := false
		// Handle wildcard or specific origin match
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				c.Writer.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				if allowedOrigin == "*" {
					// If wildcard, set Vary header if credentials are NOT allowed
					// If credentials ARE allowed (as below), wildcard origin is problematic
					// For simplicity, we might restrict '*' only if credentials are false
				}
				found = true
				break
			}
		}

		if !found {
			// Origin not allowed
			if c.Request.Method == "OPTIONS" {
				// Abort OPTIONS preflight requests from disallowed origins
				c.AbortWithStatus(http.StatusForbidden)
			} else {
				// Let other requests pass through without CORS headers
				c.Next()
			}
			return
		}

		// Common headers for allowed origins
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, Authorization, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		// Set max age for preflight requests - allow browsers to cache the preflight response
		if maxAge > 0 {
			c.Writer.Header().Set("Access-Control-Max-Age", strconv.Itoa(maxAge))
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
