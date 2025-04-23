package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/enricher"
	"github.com/orgoj/weblogproxy/internal/logger"
	"github.com/orgoj/weblogproxy/internal/rules"
	"github.com/orgoj/weblogproxy/internal/truncate"
)

// It takes the base record, rule processing result, client data (if any), request context,
// configuration, logger manager, and app logger.
func processAndLogToDestinations(
	ctx *gin.Context,
	deps struct {
		LoggerManager *logger.Manager
		Config        *config.Config
		AppLogger     *logger.AppLogger
	},
	baseRecord map[string]interface{},
	ruleResult rules.LogProcessingResult,
	clientData map[string]interface{}, // Can be nil for script downloads
) (anySuccess bool) {
	// Determine Target Destinations
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
				deps.AppLogger.Warn("Log Handler: Rule specified destination '%s' which is not enabled or configured.", name)
			}
		}
		targetDestinations = filteredDestinations
	}

	if len(targetDestinations) == 0 {
		deps.AppLogger.Warn("Log Handler: No enabled log destinations found after rule processing.")
		return false
	}

	// Process each target destination
	anySuccess = false
	for _, destName := range targetDestinations {
		loggerInstance := deps.LoggerManager.GetLogger(destName)
		if loggerInstance == nil {
			deps.AppLogger.Error("Log Handler: Logger instance '%s' not found or not initialized", destName)
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
			deps.AppLogger.Warn("Log Handler: Destination config '%s' not found after getting logger instance", destName)
		}

		// Use AccumulatedAddLogData from rules result
		finalRecord, err := enricher.EnrichAndMerge(
			destRecord,
			ruleResult.AccumulatedAddLogData,
			destAdds,
			clientData, // Pass client data here
			ctx.Request,
		)
		if err != nil {
			deps.AppLogger.Error("Log Handler: Failed to enrich/merge data for destination '%s': %v", destName, err)
			continue
		}

		limit := int64(deps.Config.Server.RequestLimits.MaxBodySize)
		if limit > 0 {
			truncated, err := truncate.TruncateMapIfNeeded(&finalRecord, limit)
			if err != nil {
				deps.AppLogger.Error("Log Handler: Failed to truncate log data for dest '%s': %v", destName, err)
			}
			if truncated && err == nil {
				deps.AppLogger.Warn("Log Handler: Log record truncated for dest '%s' due to size limit (%d bytes).", destName, limit)
			}
		}

		// Send to Logger
		if err := loggerInstance.Log(finalRecord); err != nil {
			deps.AppLogger.Error("Log Handler: Failed to write log to destination '%s': %v", destName, err)
			continue
		}

		anySuccess = true
	}

	return anySuccess
}
