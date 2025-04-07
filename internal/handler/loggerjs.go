// internal/handler/logger_js.go

package handler

import (
	"bytes"
	_ "embed" // Import the embed package
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/orgoj/weblogproxy/internal/security"
	"github.com/orgoj/weblogproxy/internal/validation"
)

// Embed the template file content into the loggerJSTemplateContent variable.
// The path is relative to the package directory.
//
//go:embed templates/logger.tmpl.js
var loggerJSTemplateContent string

// --- Data structure for the Go template ---

// LoggerJsData holds all dynamic values needed for the logger.js template.
type LoggerJsData struct {
	LogEnabled      bool
	SiteID          string
	GtmID           string
	Token           string
	LogURL          string // Full URL for the /log endpoint
	ScriptsToInject []config.ScriptInjectionSpec
}

// --- End Template Data Structure ---

// LoggerJSHandlerDeps holds dependencies for the logger.js handler.
type LoggerJSHandlerDeps struct {
	RuleProcessor      *rules.RuleProcessor
	Config             *config.Config // Need config for paths, secrets, headers
	TrustedProxies     []string       // String CIDRs/IPs
	TokenExpirationDur time.Duration
}

// Cached template instance
var (
	loggerJSTemplate *template.Template
	templateParseErr error
	parsedProxies    []*net.IPNet
)

func init() {
	loggerJSTemplate, templateParseErr = template.New("logger.js").Parse(loggerJSTemplateContent)
	if templateParseErr != nil {
		panic(fmt.Sprintf("Failed to parse embedded logger.js template: %v", templateParseErr))
	}
}

// NewLoggerJSHandler creates a Gin handler function for the /logger.js endpoint.
func NewLoggerJSHandler(deps LoggerJSHandlerDeps) gin.HandlerFunc {
	// Parse trusted proxies ONCE during handler creation
	var parseErr error
	parsedProxies, parseErr = iputil.ParseCIDRs(deps.TrustedProxies)
	if parseErr != nil {
		// Log critical error during setup
		slog.Error("Failed to parse trusted_proxies from config", "error", parseErr, "proxies", deps.TrustedProxies)
		// Return a handler that always fails, indicating setup error
		return func(ctx *gin.Context) {
			ctx.String(http.StatusInternalServerError, "Internal server configuration error (trusted_proxies)")
		}
	}

	// Check if the template was parsed successfully during init.
	if templateParseErr != nil {
		// This case should ideally be prevented by the panic in init, but keep for safety
		slog.Error("Logger.js template was not parsed successfully", "error", templateParseErr)
		return func(ctx *gin.Context) {
			ctx.String(http.StatusInternalServerError, "Internal server configuration error (template)")
		}
	}

	return func(ctx *gin.Context) {
		// 1. Get and validate input parameters
		siteID := ctx.Query("site_id")
		gtmID := ctx.Query("gtm_id") // Optional

		// Check for missing or invalid parameters
		if siteID == "" {
			slog.Warn("Missing required query parameter: site_id", "remote_ip", ctx.ClientIP(), "path", ctx.Request.URL.Path)
			// Return empty JavaScript instead of error
			executeTemplateAndRespond(ctx, LoggerJsData{})
			return
		}

		if err := validation.IsValidID(siteID, validation.DefaultMaxInputLength); err != nil {
			slog.Warn("Invalid site_id", "site_id", siteID, "error", err, "remote_ip", ctx.ClientIP())
			// Return empty JavaScript instead of error
			executeTemplateAndRespond(ctx, LoggerJsData{})
			return
		}

		if gtmID != "" {
			if err := validation.IsValidID(gtmID, validation.DefaultMaxInputLength); err != nil {
				slog.Warn("Invalid gtm_id", "gtm_id", gtmID, "error", err, "remote_ip", ctx.ClientIP())
				// Return empty JavaScript instead of error
				executeTemplateAndRespond(ctx, LoggerJsData{})
				return
			}
		}

		// Now that we have valid parameters, process the rules
		ruleResult := deps.RuleProcessor.Process(siteID, gtmID, ctx.Request)

		// Prepare data for the template in the same way regardless of whether logging is enabled
		data := LoggerJsData{
			SiteID:          siteID,
			GtmID:           gtmID,
			LogEnabled:      ruleResult.ShouldLogToServer,
			ScriptsToInject: ruleResult.AccumulatedScripts,
			Token:           "",
			LogURL:          "",
		}

		// Generate token and logURL only when logging is enabled
		if ruleResult.ShouldLogToServer {
			clientIP := iputil.GetClientIP(ctx.Request, parsedProxies)
			token, err := security.GenerateToken(deps.Config.Security.Token.Secret, siteID, gtmID, deps.TokenExpirationDur)
			if err != nil {
				// Log internal error, but continue; token will be empty
				slog.Error("Failed to generate token", "error", err, "clientIP", clientIP, "siteID", siteID, "gtm_id", gtmID)
			} else {
				data.Token = token
			}
			data.LogURL = buildLogURL(ctx, deps.Config.Server.PathPrefix, deps.Config.Server.Mode, deps.Config.Server.Domain)
		}

		// Set cache headers if configured
		for key, value := range deps.Config.Server.Headers {
			ctx.Header(key, value)
		}

		// Execute the template
		executeTemplateAndRespond(ctx, data)
	}
}

// executeTemplateAndRespond executes the template with provided data and sends the response
func executeTemplateAndRespond(ctx *gin.Context, data LoggerJsData) {
	var buf bytes.Buffer
	if err := loggerJSTemplate.Execute(&buf, data); err != nil {
		slog.Error("Failed to execute logger.js template", "error", err)
		ctx.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	ctx.Data(http.StatusOK, "application/javascript; charset=utf-8", buf.Bytes())
}

// buildLogURL constructs the URL for the /log endpoint
// Returns relative URL in embedded mode and absolute URL in standalone mode
func buildLogURL(c *gin.Context, pathPrefix string, serverMode string, serverDomain string) string {
	logPath := "/log"
	if pathPrefix != "" {
		cleanPrefix := "/" + strings.Trim(pathPrefix, "/")
		logPath = cleanPrefix + logPath
	}

	// In embedded mode we return relative URL, in standalone mode absolute
	if serverMode == "standalone" {
		// Build absolute URL from configured domain
		// If domain already contains schema (http/https), use it as is
		if strings.HasPrefix(serverDomain, "http://") || strings.HasPrefix(serverDomain, "https://") {
			return serverDomain + logPath
		}

		// Otherwise estimate schema from request
		scheme := "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}

		return scheme + "://" + serverDomain + logPath
	}

	// Embedded mode (default) - return relative URL
	return logPath
}
