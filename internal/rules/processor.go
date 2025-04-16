// internal/rules/processor.go

package rules

import (
	"fmt"
	"net"
	"net/http"

	"github.com/gobwas/glob"
	"github.com/orgoj/weblogproxy/internal/config"
	"github.com/orgoj/weblogproxy/internal/iputil" // Helper for IP/CIDR matching
)

// ProcessingResult holds the outcome of processing rules for a request.
type ProcessingResult struct {
	LogEnabled            bool
	FinalRuleMatched      bool                         // Was any rule (not ending with continue) matched?
	AccumulatedAddLogData []config.AddLogDataSpec      // Combined specs from all matched rules (last write wins)
	AccumulatedScripts    []config.ScriptInjectionSpec // Deduplicated list of scripts from all matched rules
	TargetDestinations    []string                     // From the *last* matching rule without continue (nil means all)
}

// RuleProcessor processes log rules against request parameters.
type RuleProcessor struct {
	cfg            *config.Config
	trustedProxies []*net.IPNet // Store parsed trusted proxies for reuse
}

// LogProcessingResult holds the outcome of rule processing for a request.
type LogProcessingResult struct {
	ShouldInjectScripts   bool                         // Should any scripts be injected? (Any rule matched condition)
	ShouldLogToServer     bool                         // Should active logging to /log endpoint be enabled? (Determined by the *first* final rule)
	AccumulatedScripts    []config.ScriptInjectionSpec // List of unique scripts to inject from ALL matched rules
	AccumulatedAddLogData []config.AddLogDataSpec      // Combined specs from matched rules (last write wins for a name)
	TargetDestinations    []string                     // Destinations from the *first* final rule (nil means all enabled)
}

// NewRuleProcessor creates a new RuleProcessor.
func NewRuleProcessor(cfg *config.Config) (*RuleProcessor, error) {
	trustedProxies, err := iputil.ParseCIDRs(cfg.Server.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trusted proxies: %w", err)
	}
	return &RuleProcessor{
		cfg:            cfg,
		trustedProxies: trustedProxies, // Store parsed CIDRs
	}, nil
}

// Process evaluates the configured rules against the request parameters according to the defined logic.
func (rp *RuleProcessor) Process(siteID, gtmID string, r *http.Request) LogProcessingResult {
	result := LogProcessingResult{
		ShouldInjectScripts:   false,
		ShouldLogToServer:     false, // Determined by the first final rule found, defaults to false
		AccumulatedScripts:    nil,
		AccumulatedAddLogData: nil,
		TargetDestinations:    nil,
	}
	accumulatedScriptsMap := make(map[string]config.ScriptInjectionSpec)
	accumulatedDataMap := make(map[string]config.AddLogDataSpec)

	clientIPString := iputil.GetClientIP(r, rp.trustedProxies)
	var clientIP net.IP
	if clientIPString != "" {
		clientIP = net.ParseIP(clientIPString)
	}
	userAgent := r.UserAgent()

	// Iterate through rules
	for i, rule := range rp.cfg.LogConfig {
		currentRule := rule // Use copy inside loop
		ruleID := i

		// Skip disabled rules entirely
		if !currentRule.Enabled {
			continue
		}

		if rp.matchCondition(ruleID, currentRule.Condition, siteID, gtmID, clientIP, userAgent, r) {
			// Rule condition matched
			result.ShouldInjectScripts = true // Mark that scripts might need injection

			// Accumulate AddLogData (last write wins)
			for _, spec := range currentRule.AddLogData {
				accumulatedDataMap[spec.Name] = spec
			}

			// Accumulate Scripts (deduplicate by URL)
			for _, script := range currentRule.ScriptInjection {
				if _, exists := accumulatedScriptsMap[script.URL]; !exists {
					accumulatedScriptsMap[script.URL] = script
				}
			}

			// Check if this is a final rule (not continuing)
			if !currentRule.Continue {
				// This is a final rule - set logging decision and destinations
				result.ShouldLogToServer = true
				if len(currentRule.LogDestinations) > 0 {
					result.TargetDestinations = currentRule.LogDestinations
				} else {
					result.TargetDestinations = nil // Explicitly nil means all enabled
				}
				break // Stop processing further rules
			}
			// For continue rules, just keep accumulating values
		}
	}

	// Convert accumulated maps to slices (always do this, scripts might be injected even if logging is off)
	if len(accumulatedScriptsMap) > 0 {
		result.AccumulatedScripts = make([]config.ScriptInjectionSpec, 0, len(accumulatedScriptsMap))
		for _, script := range accumulatedScriptsMap {
			result.AccumulatedScripts = append(result.AccumulatedScripts, script)
		}
	}
	if len(accumulatedDataMap) > 0 {
		result.AccumulatedAddLogData = make([]config.AddLogDataSpec, 0, len(accumulatedDataMap))
		for _, dataSpec := range accumulatedDataMap {
			result.AccumulatedAddLogData = append(result.AccumulatedAddLogData, dataSpec)
		}
	}

	return result
}

// matchCondition checks if the request parameters match the rule's condition.
// Added ruleID for logging purposes.
func (rp *RuleProcessor) matchCondition(ruleID int, cond config.LogRuleCondition, siteID, gtmID string, clientIP net.IP, userAgent string, r *http.Request) bool {
	// fmt.Printf("[DEBUG] matchCondition: Evaluating Rule %d, Condition: %+v, Inputs: siteID='%s', gtmID='%s', clientIP='%v', userAgent='%s'\n", ruleID, cond, siteID, gtmID, clientIP, userAgent)
	emptyCondition := cond.SiteID == "" && len(cond.GTMIDs) == 0 && len(cond.UserAgents) == 0 && len(cond.IPs) == 0 && len(cond.Headers) == 0
	if emptyCondition {
		// fmt.Printf("[DEBUG] matchCondition SUCCESS Rule %d: Empty condition\n", ruleID)
		return true
	}

	// --- Perform checks ONLY if the corresponding condition field is defined ---

	// SiteID check
	if cond.SiteID != "" {
		if cond.SiteID != siteID {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=SiteID mismatch, Input='%s', Condition='%s'\n", ruleID, siteID, cond.SiteID)
			return false
		}
	}

	// GTMIDs check
	if len(cond.GTMIDs) > 0 {
		match := false
		for _, ruleGtmID := range cond.GTMIDs {
			if ruleGtmID == gtmID {
				match = true
				break
			}
		}
		if !match {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=GTMID mismatch, Input='%s', Condition=%v\n", ruleID, gtmID, cond.GTMIDs)
			return false
		}
	}

	// --- Headers check ---
	if len(cond.Headers) > 0 {
		if r == nil {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=Headers condition present but no request\n", ruleID)
			return false
		}

		for headerName, headerCondition := range cond.Headers {
			// Processing false value means the header must be absent
			if headerCondition == false {
				if r.Header.Get(headerName) != "" {
					// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=Header '%s' should be absent but is present\n", ruleID, headerName)
					return false
				}
				continue
			}

			// Value true means the header must exist (regardless of its value)
			if headerCondition == true {
				if r.Header.Get(headerName) == "" {
					// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=Header '%s' should be present but is absent\n", ruleID, headerName)
					return false
				}
				continue
			}

			// Check for specific header value
			expectedValue, ok := headerCondition.(string)
			if ok {
				actualValue := r.Header.Get(headerName)
				if actualValue != expectedValue {
					// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=Header '%s' value mismatch, Expected='%s', Got='%s'\n", ruleID, headerName, expectedValue, actualValue)
					return false
				}
			} else {
				// Unsupported value type in condition
				// fmt.Printf("[WARN] matchCondition Rule %d: Unsupported header condition value type for '%s': %T\n", ruleID, headerName, headerCondition)
				return false
			}
		}
	}

	// --- UserAgents and IPs checks moved to the end ---

	// UserAgents check
	if len(cond.UserAgents) > 0 {
		match := false
		for _, pattern := range cond.UserAgents {
			g, err := glob.Compile(pattern)
			if err != nil {
				// fmt.Printf("[WARN] matchCondition Rule %d: Invalid UserAgent glob pattern '%s': %v\n", ruleID, pattern, err)
				continue // Skip invalid patterns
			}
			if g.Match(userAgent) {
				match = true
				break
			}
		}
		if !match {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=UserAgent mismatch, Input='%s', Condition=%v\n", ruleID, userAgent, cond.UserAgents)
			return false
		}
	}

	// IPs check
	if len(cond.IPs) > 0 {
		if clientIP == nil {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=IP mismatch (client IP is nil), Condition=%v\n", ruleID, cond.IPs)
			return false
		}
		cidrs, err := iputil.ParseCIDRs(cond.IPs)
		if err != nil {
			// fmt.Printf("[WARN] matchCondition Rule %d: Invalid IP CIDR patterns %v: %v\n", ruleID, cond.IPs, err)
			return false // Don't match if patterns are invalid
		}
		if !iputil.IsIPInAnyCIDR(clientIP, cidrs) {
			// fmt.Printf("[DEBUG] matchCondition FAIL Rule %d: Reason=IP mismatch (IP %v not in CIDRs), Condition=%v\n", ruleID, clientIP, cond.IPs)
			return false
		}
	}

	// If we haven't returned false yet, all defined and checked conditions matched
	// fmt.Printf("[DEBUG] matchCondition SUCCESS Rule %d: All conditions met\n", ruleID)
	return true
}
