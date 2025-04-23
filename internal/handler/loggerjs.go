// internal/handler/logger_js.go

package handler

import (
	"bytes"
	_ "embed" // Import the embed package
	"fmt"
	"html/template"
	"net"
	"net/http"
	"strings"
	"time"

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

// Embed the template file content into the loggerJSTemplateContent variable.
// The path is relative to the package directory.
//
//go:embed templates/logger.tmpl.js
var loggerJSTemplateContent string

// --- Data structure for the Go template ---

// LoggerJsData holds all dynamic values needed for the logger.js template.
type LoggerJsData struct {
	LogEnabled        bool
	SiteID            string
	GtmID             string
	Token             string
	LogURL            string // Full URL for the /log endpoint
	ScriptsToInject   []ScriptInjectionTemplateData
	GlobalObjectName  string // Name of the global JavaScript object
	JavaScriptOptions struct {
		TrackURL       bool
		TrackTraceback bool
	}
}

// ScriptInjectionTemplateData extends ScriptInjectionSpec with template-specific fields
type ScriptInjectionTemplateData struct {
	config.ScriptInjectionSpec
	IsLast bool
}

// --- End Template Data Structure ---

// LoggerJSHandlerDeps holds dependencies for the logger.js handler.
type LoggerJSHandlerDeps struct {
	RuleProcessor      *rules.RuleProcessor
	Config             *config.Config // Need config for paths, secrets, headers
	TrustedProxies     []string       // String CIDRs/IPs
	TokenExpirationDur time.Duration
	AppLogger          *logger.AppLogger
	LoggerManager      *logger.Manager
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
	// Check if the AppLogger exists
	if deps.AppLogger == nil {
		panic("LoggerJSHandler requires a non-nil AppLogger")
	}
	if deps.LoggerManager == nil {
		panic("LoggerJSHandler requires a non-nil LoggerManager")
	}

	// Parse trusted proxies ONCE during handler creation
	var parseErr error
	parsedProxies, parseErr = iputil.ParseCIDRs(deps.TrustedProxies)
	if parseErr != nil {
		// Log critical error during setup
		deps.AppLogger.Error("Failed to parse trusted_proxies from config: %v, proxies: %v", parseErr, deps.TrustedProxies)
		// Return a handler that always fails, indicating setup error
		return func(ctx *gin.Context) {
			ctx.String(http.StatusInternalServerError, "Internal server configuration error (trusted_proxies)")
		}
	}

	// Check if the template was parsed successfully during init.
	if templateParseErr != nil {
		// This case should ideally be prevented by the panic in init, but keep for safety
		deps.AppLogger.Error("Logger.js template was not parsed successfully: %v", templateParseErr)
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
			deps.AppLogger.Warn("Missing required query parameter: site_id, remote_ip: %s, path: %s", ctx.ClientIP(), ctx.Request.URL.Path)
			// Return empty JavaScript instead of error
			executeTemplateAndRespond(ctx, LoggerJsData{
				GlobalObjectName: deps.Config.Server.JavaScript.GlobalObjectName,
			}, deps.AppLogger)
			return
		}

		if err := validation.IsValidID(siteID, validation.DefaultMaxInputLength); err != nil {
			deps.AppLogger.Warn("Invalid site_id: %s, error: %v, remote_ip: %s", siteID, err, ctx.ClientIP())
			// Return empty JavaScript instead of error
			executeTemplateAndRespond(ctx, LoggerJsData{
				GlobalObjectName: deps.Config.Server.JavaScript.GlobalObjectName,
			}, deps.AppLogger)
			return
		}

		if gtmID != "" {
			if err := validation.IsValidID(gtmID, validation.DefaultMaxInputLength); err != nil {
				deps.AppLogger.Warn("Invalid gtm_id: %s, error: %v, remote_ip: %s", gtmID, err, ctx.ClientIP())
				// Return empty JavaScript instead of error
				executeTemplateAndRespond(ctx, LoggerJsData{
					GlobalObjectName: deps.Config.Server.JavaScript.GlobalObjectName,
				}, deps.AppLogger)
				return
			}
		}

		// Now that we have valid parameters, process the rules
		ruleResult := deps.RuleProcessor.Process(siteID, gtmID, ctx.Request)

		// Log script download if enabled
		if ruleResult.ShouldLogScriptDownloads {
			// Create base record with script download specific fields
			baseRecord := enricher.CreateBaseRecord(siteID, gtmID, iputil.GetClientIP(ctx.Request, parsedProxies, deps.Config.Server.ClientIPHeader))
			baseRecord["msg"] = "logger.js download"
			baseRecord["event_type"] = "script_download"
			baseRecord["script_type"] = "logger"

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
						deps.AppLogger.Warn("Logger.js Handler: Rule specified destination '%s' which is not enabled or configured.", name)
					}
				}
				targetDestinations = filteredDestinations
			}

			if len(targetDestinations) == 0 {
				deps.AppLogger.Warn("Logger.js Handler: No enabled log destinations found after rule processing.")
				return
			}

			// 5. Process each target destination
			anySuccess := false
			for _, destName := range targetDestinations {
				loggerInstance := deps.LoggerManager.GetLogger(destName)
				if loggerInstance == nil {
					deps.AppLogger.Error("Logger.js Handler: Logger instance '%s' not found or not initialized for SiteID '%s'", destName, siteID)
					continue
				}

				// Create a fresh copy of the base record for this destination
				destRecord := make(map[string]interface{}, len(baseRecord))
				for k, v := range baseRecord {
					destRecord[k] = v
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
					deps.AppLogger.Warn("Logger.js Handler: Destination config '%s' not found for SiteID '%s' after getting logger instance", destName, siteID)
				}

				// Use AccumulatedAddLogData from rules result
				finalRecord, err := enricher.EnrichAndMerge(
					destRecord,
					ruleResult.AccumulatedAddLogData,
					destAdds,
					nil,
					ctx.Request,
				)
				if err != nil {
					deps.AppLogger.Error("Logger.js Handler: Failed to enrich/merge data for destination '%s', SiteID '%s': %v", destName, siteID, err)
					continue
				}

				limit := int64(deps.Config.Server.RequestLimits.MaxBodySize)
				if limit > 0 {
					truncated, err := truncate.TruncateMapIfNeeded(&finalRecord, limit)
					if err != nil {
						deps.AppLogger.Error("Logger.js Handler: Failed to truncate log data for dest '%s', SiteID '%s': %v", destName, siteID, err)
					}
					if truncated && err == nil {
						deps.AppLogger.Warn("Logger.js Handler: Log record truncated for dest '%s', SiteID '%s' due to size limit (%d bytes).", destName, siteID, limit)
					}
				}

				// 6. Send to Logger
				if err := loggerInstance.Log(finalRecord); err != nil {
					deps.AppLogger.Error("Logger.js Handler: Failed to write log to destination '%s' for SiteID '%s': %v", destName, siteID, err)
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
		}

		// Prepare data for the template in the same way regardless of whether logging is enabled
		data := LoggerJsData{
			SiteID:            siteID,
			GtmID:             gtmID,
			LogEnabled:        ruleResult.ShouldLogToServer,
			Token:             "",
			LogURL:            "",
			GlobalObjectName:  deps.Config.Server.JavaScript.GlobalObjectName,
			JavaScriptOptions: ruleResult.AccumulatedJavaScriptOptions,
		}

		// Convert ScriptInjectionSpec to ScriptInjectionTemplateData with IsLast flag
		if len(ruleResult.AccumulatedScripts) > 0 {
			data.ScriptsToInject = make([]ScriptInjectionTemplateData, len(ruleResult.AccumulatedScripts))
			for i, script := range ruleResult.AccumulatedScripts {
				data.ScriptsToInject[i] = ScriptInjectionTemplateData{
					ScriptInjectionSpec: script,
					IsLast:              i == len(ruleResult.AccumulatedScripts)-1,
				}
			}
		}

		// Generate token and logURL only when logging is enabled
		if ruleResult.ShouldLogToServer {
			clientIP := iputil.GetClientIP(ctx.Request, parsedProxies, deps.Config.Server.ClientIPHeader)
			token, err := security.GenerateToken(deps.Config.Security.Token.Secret, siteID, gtmID, deps.TokenExpirationDur)
			if err != nil {
				// Log internal error, but continue; token will be empty
				deps.AppLogger.Error("Failed to generate token: %v, clientIP: %s, siteID: %s, gtm_id: %s", err, clientIP, siteID, gtmID)
			} else {
				data.Token = token
			}
			data.LogURL = buildLogURL(ctx, deps.Config.Server.PathPrefix, deps.Config.Server.Mode, deps.Config.Server.Domain, deps.Config.Server.Protocol)
		}

		// Set cache headers if configured
		for key, value := range deps.Config.Server.Headers {
			ctx.Header(key, value)
		}

		// Execute the template
		executeTemplateAndRespond(ctx, data, deps.AppLogger)
	}
}

// executeTemplateAndRespond executes the template with provided data and sends the response
func executeTemplateAndRespond(ctx *gin.Context, data LoggerJsData, appLogger *logger.AppLogger) {
	var buf bytes.Buffer
	if err := loggerJSTemplate.Execute(&buf, data); err != nil {
		appLogger.Error("Failed to execute logger.js template: %v", err)
		ctx.String(http.StatusInternalServerError, "Internal Server Error")
		return
	}

	ctx.Data(http.StatusOK, "application/javascript; charset=utf-8", buf.Bytes())
}

// buildLogURL constructs the URL for the /log endpoint
// Returns relative URL in embedded mode and absolute URL in standalone mode
func buildLogURL(c *gin.Context, pathPrefix string, serverMode string, serverDomain string, protocol string) string {
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

		// Use configured protocol
		return protocol + "://" + serverDomain + logPath
	}

	// Embedded mode (default) - return relative URL
	return logPath
}
