package server

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"log/slog"
	"os"

	"github.com/gin-gonic/gin"
	configparser "github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/handler"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	sloggin "github.com/samber/slog-gin"
	"golang.org/x/time/rate"
)

// Dependencies holds the dependencies needed by the server.
type Dependencies struct {
	Config        *configparser.Config
	LoggerManager *logger.Manager
	RuleProcessor *rules.RuleProcessor
	AppLogger     *logger.AppLogger
}

// Server represents the HTTP server
type Server struct {
	router        *gin.Engine
	config        *configparser.Config
	loggerManager *logger.Manager
	ruleProcessor *rules.RuleProcessor
	// Rate limiting specific
	limiters             map[string]*rate.Limiter
	limiterMu            sync.Mutex
	rateLimit            rate.Limit
	burstLimit           int
	trustedProxiesParsed []*net.IPNet
	healthAllowed        []*net.IPNet
	deps                 Dependencies
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
	if deps.AppLogger == nil {
		panic("server: AppLogger dependency cannot be nil")
	}

	deps.AppLogger.Debug("Creating server with mode: %s, path_prefix: %s", deps.Config.Server.Mode, deps.Config.Server.PathPrefix)

	// Set Gin mode (always release)
	gin.SetMode(gin.ReleaseMode)

	router := gin.New()
	// Moderní logging přes slog-gin
	var slogLevel slog.Level
	switch deps.Config.AppLog.Level {
	case "TRACE":
		slogLevel = slog.LevelDebug // slog nemá TRACE, použijeme DEBUG
	case "DEBUG":
		slogLevel = slog.LevelDebug
	case "INFO":
		slogLevel = slog.LevelInfo
	case "WARN":
		slogLevel = slog.LevelWarn
	case "ERROR":
		slogLevel = slog.LevelError
	case "FATAL":
		slogLevel = slog.LevelError // slog nemá FATAL, použijeme ERROR
	default:
		slogLevel = slog.LevelWarn
	}
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slogLevel}))

	slogGinConfig := sloggin.Config{
		DefaultLevel:     slog.LevelInfo, // Default level for requests is INFO unless overridden
		ClientErrorLevel: slog.LevelWarn,
		ServerErrorLevel: slog.LevelError,
		// Add a filter to skip /health logs if show_health_logs is false
		Filters: []sloggin.Filter{
			func(c *gin.Context) bool {
				if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/health/" {
					// Check both base path and prefixed path if applicable
					basePath := "/"
					if deps.Config.Server.Mode == "embedded" && deps.Config.Server.PathPrefix != "" {
						basePath = deps.Config.Server.PathPrefix
						if basePath[0] != '/' {
							basePath = "/" + basePath
						}
						if basePath != "/" && basePath[len(basePath)-1] != '/' {
							basePath += "/"
						}
					}
					healthPath := basePath + "health"
					if c.Request.URL.Path == healthPath || c.Request.URL.Path == healthPath+"/" {
						return !deps.Config.AppLog.ShowHealthLogs // Skip if ShowHealthLogs is false
					}
				}
				return true // Log other requests
			},
		},
	}

	// Adjust default level based on global config if it's higher than INFO
	if slogLevel > slog.LevelInfo {
		slogGinConfig.DefaultLevel = slogLevel
	}

	router.Use(sloggin.NewWithConfig(logger, slogGinConfig))
	router.Use(gin.Recovery())

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

	// Parse trusted_proxies and exit on invalid config
	parsedTrusted, err := iputil.ParseCIDRs(deps.Config.Server.TrustedProxies)
	if err != nil {
		panic(fmt.Sprintf("server: invalid server.trusted_proxies: %v", err))
	}
	server.trustedProxiesParsed = parsedTrusted
	// Parse health_allowed_ips and exit on invalid config
	parsedHealthAllowed, err := iputil.ParseCIDRs(deps.Config.Server.HealthAllowedIPs)
	if err != nil {
		panic(fmt.Sprintf("server: invalid server.health_allowed_ips: %v", err))
	}
	server.healthAllowed = parsedHealthAllowed

	// Initialize rate limiter settings
	if deps.Config.Server.RequestLimits.RateLimit > 0 {
		// Convert requests per minute to requests per second
		server.rateLimit = rate.Limit(float64(deps.Config.Server.RequestLimits.RateLimit) / 60.0)
		// Set burst limit (e.g., allow bursts up to the per-minute limit)
		server.burstLimit = deps.Config.Server.RequestLimits.RateLimit
		deps.AppLogger.Info("Rate limiting enabled for /log: Rate=%.2f req/sec, Burst=%d", server.rateLimit, server.burstLimit)
	} else {
		server.rateLimit = rate.Inf // No limit
		server.burstLimit = 0
		deps.AppLogger.Info("Rate limiting disabled for /log.")
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

	s.deps.AppLogger.Debug("Setting up routes with basePath: %s", basePath)

	group := s.router.Group(basePath)
	{
		// Health check endpoint (no rate limit)
		group.GET("health", s.healthIPMiddleware(), func(c *gin.Context) {
			s.deps.AppLogger.Health("Health endpoint called with method %s, path %s", c.Request.Method, c.Request.URL.Path)
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		})
		group.HEAD("health", s.healthIPMiddleware(), func(c *gin.Context) {
			s.deps.AppLogger.Health("Health endpoint called with method %s, path %s", c.Request.Method, c.Request.URL.Path)
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
			AppLogger:          s.deps.AppLogger,
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
				AppLogger:      s.deps.AppLogger,
			}

			// Register Log Handler
			logGroup.POST("", handler.NewLogHandler(logDeps)) // POST /log
		}
	}

	// Nastavím NoRoute handler
	s.router.NoRoute(func(c *gin.Context) {
		c.Header("Cache-Control", s.config.Server.UnknownRoute.CacheControl)
		c.Status(s.config.Server.UnknownRoute.Code)
		_, _ = c.Writer.Write([]byte(""))
	})
}

// rateLimitMiddleware creates a Gin middleware for rate limiting based on IP.
func (s *Server) rateLimitMiddleware() gin.HandlerFunc {
	// Pre-parse trusted proxies once for the middleware
	parsedTrustedProxies, err := iputil.ParseCIDRs(s.config.Server.TrustedProxies)
	if err != nil {
		// Log critical error during server setup
		s.deps.AppLogger.Error("Failed to parse trusted proxies for rate limiter: %v", err)
		// Return a middleware that always denies? Or panic?
		return func(c *gin.Context) {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Internal server error (rate limiter config)"})
		}
	}

	return func(c *gin.Context) {
		ip := iputil.GetClientIP(c.Request, parsedTrustedProxies, s.config.Server.ClientIPHeader)

		s.limiterMu.Lock()
		limiter, exists := s.limiters[ip]
		if !exists {
			limiter = rate.NewLimiter(s.rateLimit, s.burstLimit)
			s.limiters[ip] = limiter
		}
		s.limiterMu.Unlock()

		if !limiter.Allow() {
			// Log the rate limit exceedance internally
			s.deps.AppLogger.Info("Rate limit exceeded for IP: %s", ip)
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Rate limit exceeded"})
			return
		}

		c.Next()
	}
}

// healthIPMiddleware checks client IP against allowed CIDRs for health endpoints
func (s *Server) healthIPMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		ipStr := iputil.GetClientIP(c.Request, s.trustedProxiesParsed, s.config.Server.ClientIPHeader)
		ip := net.ParseIP(ipStr)
		if ip == nil {
			s.deps.AppLogger.Error("Failed to parse client IP for health check: %s", ipStr)
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		if len(s.healthAllowed) > 0 && !iputil.IsIPInAnyCIDR(ip, s.healthAllowed) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden"})
			return
		}
		c.Next()
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	addr := fmt.Sprintf("%s:%d", s.config.Server.Host, s.config.Server.Port)
	s.deps.AppLogger.Warn("Starting server on %s", addr)
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
