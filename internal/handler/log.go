// internal/handler/log.go

package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/enricher"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/orgoj/weblogproxy/internal/security"
	"github.com/orgoj/weblogproxy/internal/validation"
)

// LogRequestBody defines the structure for the /log endpoint request body
type LogRequestBody struct {
	Token  string                 `json:"token" binding:"required"`
	SiteID string                 `json:"site_id" binding:"required"`
	GtmID  string                 `json:"gtm_id"` // Optional
	Data   map[string]interface{} `json:"data" binding:"required"`
}

// LogHandlerDependencies holds dependencies for the log handler
type LogHandlerDependencies struct {
	LoggerManager  *logger.Manager
	TokenSecret    string
	RuleProcessor  *rules.RuleProcessor
	TrustedProxies []string
	Config         *config.Config
	AppLogger      *logger.AppLogger
}

// NewLogHandler creates a Gin handler function for the /log endpoint
func NewLogHandler(deps LogHandlerDependencies) gin.HandlerFunc {

	// Check for nil dependencies early (optional but good practice)

	if deps.LoggerManager == nil {
		panic("LogHandler requires a non-nil LoggerManager")
	}
	if deps.RuleProcessor == nil {
		panic("LogHandler requires a non-nil RuleProcessor")
	}
	if deps.Config == nil {
		panic("LogHandler requires a non-nil Config")
	}
	if deps.AppLogger == nil {
		panic("LogHandler requires a non-nil AppLogger")
	}

	parsedTrustedProxies, err := iputil.ParseCIDRs(deps.TrustedProxies)
	if err != nil {
		deps.AppLogger.Error("Failed to parse trusted proxies for log handler: %v", err)
	}

	return func(ctx *gin.Context) {
		// Always set status OK first, change only on specific errors like Rate Limit
		ctx.Status(http.StatusOK)

		// Limit request body size BEFORE parsing JSON
		if deps.Config.Server.RequestLimits.MaxBodySize > 0 {
			ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, int64(deps.Config.Server.RequestLimits.MaxBodySize))
		}

		var reqBody LogRequestBody
		if err := ctx.ShouldBindJSON(&reqBody); err != nil {
			// Log the binding error internally but return OK to client
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
			deps.AppLogger.Warn("Log Handler: JSON binding error for IP %s: %v", clientIPForLog, err)
			// Maybe truncate the body before logging if it was too large?
			// For now, just log the error.
			return // StatusOK is already set
		}

		// --- Input Validation & Sanitization ---
		// Validate SiteID and GtmID format
		if err := validation.IsValidID(reqBody.SiteID, validation.DefaultMaxInputLength); err != nil {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
			deps.AppLogger.Warn("Log Handler: Invalid site_id '%s' from IP %s: %v", reqBody.SiteID, clientIPForLog, err)
			// Do not process further, but return OK
			return
		}
		if reqBody.GtmID != "" {
			if err := validation.IsValidID(reqBody.GtmID, validation.DefaultMaxInputLength); err != nil {
				clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
				deps.AppLogger.Warn("Log Handler: Invalid gtm_id '%s' from IP %s: %v", reqBody.GtmID, clientIPForLog, err)
				// Do not process further, but return OK
				return
			}
		}

		// Sanitize the client-provided data map
		sanitizedData, err := validation.SanitizeMapRecursively(
			reqBody.Data,
			validation.DefaultMaxDepth,
			0, // Start at depth 0
			validation.DefaultMaxKeyLength,
			validation.DefaultMaxInputLength, // Reuse max input length for strings for now
		)
		if err != nil {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
			deps.AppLogger.Warn("Log Handler: Data sanitization error for IP %s (SiteID: %s): %v", clientIPForLog, reqBody.SiteID, err)
			// Decide if we still want to log the (partially?) sanitized data or skip.
			// For now, let's skip if sanitization fails completely.
			if sanitizedData == nil {
				return // Return OK
			}
			// If partially sanitized, log the warning and continue with what we have.
		}
		// Use sanitizedData from now on
		reqBody.Data = sanitizedData
		// --- End Input Validation & Sanitization ---

		// 1. Verify Token - Use security.ValidateToken directly
		valid, err := security.ValidateToken(deps.TokenSecret, reqBody.SiteID, reqBody.GtmID, reqBody.Token)
		if err != nil {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
			deps.AppLogger.Warn("Log Handler: Token validation error for IP %s, SiteID '%s': %v", clientIPForLog, reqBody.SiteID, err)
			ctx.Header("X-Log-Status", "failure")
			return // Return OK, but log the error
		}
		if !valid {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
			deps.AppLogger.Warn("Log Handler: Invalid token received from IP %s for SiteID '%s'", clientIPForLog, reqBody.SiteID)
			ctx.Header("X-Log-Status", "failure")
			return // Return OK
		}

		// 2. Process Rules
		ruleResult := deps.RuleProcessor.Process(reqBody.SiteID, reqBody.GtmID, ctx.Request)

		// 3. If logging disabled by rules, stop here
		if !ruleResult.ShouldLogToServer {
			return
		}

		// 4. Determine Target Destinations and Log
		clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies, deps.Config.Server.ClientIPHeader)
		baseRecordTemplate := enricher.CreateBaseRecord(reqBody.SiteID, reqBody.GtmID, clientIPForLog)

		logDeps := struct {
			LoggerManager *logger.Manager
			Config        *config.Config
			AppLogger     *logger.AppLogger
		}{
			deps.LoggerManager,
			deps.Config,
			deps.AppLogger,
		}

		anySuccess := processAndLogToDestinations(ctx, logDeps, baseRecordTemplate, ruleResult, reqBody.Data)

		// Set response headers for successful logging
		if anySuccess {
			ctx.Header("X-Log-Status", "success")
		} else {
			ctx.Header("X-Log-Status", "error")
		}

		// All processing done, return
	}
}
