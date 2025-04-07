// internal/handler/log.go

package handler

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/enricher"
	"github.com/orgoj/weblogproxy/internal/iputil"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/orgoj/weblogproxy/internal/security"
	"github.com/orgoj/weblogproxy/internal/truncate"
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

	parsedTrustedProxies, err := iputil.ParseCIDRs(deps.TrustedProxies)
	if err != nil {
		fmt.Printf("[CRITICAL] Failed to parse trusted proxies for log handler: %v\n", err)
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
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
			fmt.Printf("[WARN] Log Handler: JSON binding error for IP %s: %v\n", clientIPForLog, err)
			// Maybe truncate the body before logging if it was too large?
			// For now, just log the error.
			return // StatusOK is already set
		}

		// --- Input Validation & Sanitization ---
		// Validate SiteID and GtmID format
		if err := validation.IsValidID(reqBody.SiteID, validation.DefaultMaxInputLength); err != nil {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
			fmt.Printf("[WARN] Log Handler: Invalid site_id '%s' from IP %s: %v\n", reqBody.SiteID, clientIPForLog, err)
			// Do not process further, but return OK
			return
		}
		if reqBody.GtmID != "" {
			if err := validation.IsValidID(reqBody.GtmID, validation.DefaultMaxInputLength); err != nil {
				clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
				fmt.Printf("[WARN] Log Handler: Invalid gtm_id '%s' from IP %s: %v\n", reqBody.GtmID, clientIPForLog, err)
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
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
			fmt.Printf("[WARN] Log Handler: Data sanitization error for IP %s (SiteID: %s): %v\n", clientIPForLog, reqBody.SiteID, err)
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
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
			fmt.Printf("[WARN] Log Handler: Token validation error for IP %s, SiteID '%s': %v\n", clientIPForLog, reqBody.SiteID, err)
			ctx.Header("X-Log-Status", "failure")
			return // Return OK, but log the error
		}
		if !valid {
			clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
			fmt.Printf("[WARN] Log Handler: Invalid token received from IP %s for SiteID '%s'\n", clientIPForLog, reqBody.SiteID)
			ctx.Header("X-Log-Status", "failure")
			return // Return OK
		}

		// 2. Process Rules
		ruleResult := deps.RuleProcessor.Process(reqBody.SiteID, reqBody.GtmID, ctx.Request)

		// 3. If logging disabled by rules, stop here
		if !ruleResult.ShouldLogToServer {
			return
		}

		// 4. Determine Target Destinations
		var targetDestinations []string
		if ruleResult.TargetDestinations == nil {
			// No specific destinations from the final rule, use all enabled
			targetDestinations = deps.LoggerManager.GetAllEnabledLoggerNames()
		} else {
			// Use the specific destinations from the final rule
			// We still need to ensure these destinations are actually enabled in the LoggerManager
			enabledLoggers := deps.LoggerManager.GetAllEnabledLoggerNames()
			enabledMap := make(map[string]bool)
			for _, name := range enabledLoggers {
				enabledMap[name] = true
			}
			filteredDestinations := make([]string, 0, len(ruleResult.TargetDestinations))
			for _, name := range ruleResult.TargetDestinations {
				if enabledMap[name] {
					filteredDestinations = append(filteredDestinations, name)
				} else {
					fmt.Printf("[WARN] Log Handler: Rule specified destination '%s' which is not enabled or configured.\n", name)
				}
			}
			targetDestinations = filteredDestinations
		}

		if len(targetDestinations) == 0 {
			fmt.Println("[WARN] Log Handler: No enabled log destinations found after rule processing.")
			return
		}

		// 5. Process each target destination
		clientIPForLog := iputil.GetClientIP(ctx.Request, parsedTrustedProxies)
		userAgentForLog := ctx.Request.UserAgent()
		baseRecordTemplate := enricher.CreateBaseRecord(reqBody.SiteID, reqBody.GtmID, clientIPForLog, userAgentForLog)

		// 6. Send to Logger
		anySuccess := false
		for _, destName := range targetDestinations {
			loggerInstance := deps.LoggerManager.GetLogger(destName)
			if loggerInstance == nil {
				fmt.Printf("[ERROR] Log Handler: Logger instance '%s' not found or not initialized for SiteID '%s'\n", destName, reqBody.SiteID)
				continue
			}

			// Create a fresh copy of the base record for this destination
			baseRecord := make(map[string]interface{}, len(baseRecordTemplate))
			for k, v := range baseRecordTemplate {
				baseRecord[k] = v
			}

			var destConfig *config.LogDestination
			for i := range deps.Config.LogDestinations {
				if deps.Config.LogDestinations[i].Name == destName && deps.Config.LogDestinations[i].Enabled {
					destConfig = &deps.Config.LogDestinations[i]
					break
				}
			}

			var destAdds []config.AddLogDataSpec
			if destConfig != nil {
				destAdds = destConfig.AddLogData
			} else {
				fmt.Printf("[WARN] Log Handler: Destination config '%s' not found for SiteID '%s' after getting logger instance\n", destName, reqBody.SiteID)
			}

			// Use AccumulatedAddLogData from rules result
			finalRecord, err := enricher.EnrichAndMerge(
				baseRecord,
				ruleResult.AccumulatedAddLogData,
				destAdds,
				reqBody.Data,
				ctx.Request,
			)
			if err != nil {
				fmt.Printf("[ERROR] Log Handler: Failed to enrich/merge data for destination '%s', SiteID '%s': %v\n", destName, reqBody.SiteID, err)
				continue
			}

			limit := int64(deps.Config.Server.RequestLimits.MaxBodySize)
			if limit > 0 {
				truncated, err := truncate.TruncateMapIfNeeded(&finalRecord, limit)
				if err != nil {
					fmt.Printf("[ERROR] Log Handler: Failed to truncate log data for dest '%s', SiteID '%s': %v\n", destName, reqBody.SiteID, err)
				}
				if truncated && err == nil {
					fmt.Printf("[WARN] Log Handler: Log record truncated for dest '%s', SiteID '%s' due to size limit (%d bytes).\n", destName, reqBody.SiteID, limit)
				}
			}

			// 6. Send to Logger
			if err := loggerInstance.Log(finalRecord); err != nil {
				fmt.Printf("[ERROR] Log Handler: Failed to write log to destination '%s' for SiteID '%s': %v\n", destName, reqBody.SiteID, err)
				continue
			}

			anySuccess = true
		}

		// Set response headers for successful logging
		if anySuccess {
			ctx.Header("X-Log-Status", "success")
		} else {
			ctx.Header("X-Log-Status", "error")
		}

		// All processing done, return
		return
	}
}

// Helper function to find destination config by name
func findDestinationConfig(destinations []config.LogDestination, name string) (config.LogDestination, bool) {
	for _, dest := range destinations {
		if dest.Name == name {
			return dest, true
		}
	}
	return config.LogDestination{}, false
}
