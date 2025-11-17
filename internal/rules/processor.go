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

// compiledCondition holds pre-compiled patterns for efficient matching
type compiledCondition struct {
	siteID         string
	gtmIDs         []string
	userAgentGlobs []glob.Glob         // Pre-compiled glob patterns
	ipCIDRs        []*net.IPNet        // Pre-parsed IP/CIDR ranges
	headers        map[string]interface{}
}

// compiledRule holds a rule with its pre-compiled condition
type compiledRule struct {
	rule      config.LogRule
	condition compiledCondition
}

// RuleProcessor processes log rules against request parameters.
type RuleProcessor struct {
	cfg            *config.Config
	trustedProxies []*net.IPNet      // Store parsed trusted proxies for reuse
	compiledRules  []compiledRule    // Pre-compiled rules for performance
}

// LogProcessingResult holds the outcome of rule processing for a request.
type LogProcessingResult struct {
	ShouldInjectScripts          bool                         // Should any scripts be injected? (Any rule matched condition)
	ShouldLogToServer            bool                         // Should active logging to /log endpoint be enabled? (Determined by the *first* final rule)
	ShouldLogScriptDownloads     bool                         // Should log script downloads? (Determined by the *first* final rule)
	AccumulatedScripts           []config.ScriptInjectionSpec // List of unique scripts to inject from ALL matched rules
	AccumulatedAddLogData        []config.AddLogDataSpec      // Combined specs from matched rules (last write wins for a name)
	TargetDestinations           []string                     // Destinations from the *first* final rule (nil means all enabled)
	AccumulatedJavaScriptOptions struct {                     // JavaScript options from matched rules (last write wins)
		TrackURL       bool
		TrackTraceback bool
	}
}

// NewRuleProcessor creates a new RuleProcessor with pre-compiled patterns.
func NewRuleProcessor(cfg *config.Config) (*RuleProcessor, error) {
	trustedProxies, err := iputil.ParseCIDRs(cfg.Server.TrustedProxies)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trusted proxies: %w", err)
	}

	// Pre-compile all rules and their patterns
	compiledRules := make([]compiledRule, 0, len(cfg.LogConfig))
	for i, rule := range cfg.LogConfig {
		compiled := compiledRule{
			rule: rule,
			condition: compiledCondition{
				siteID:  rule.Condition.SiteID,
				gtmIDs:  rule.Condition.GTMIDs,
				headers: rule.Condition.Headers,
			},
		}

		// Pre-compile user agent glob patterns
		if len(rule.Condition.UserAgents) > 0 {
			compiled.condition.userAgentGlobs = make([]glob.Glob, 0, len(rule.Condition.UserAgents))
			for _, pattern := range rule.Condition.UserAgents {
				g, err := glob.Compile(pattern)
				if err != nil {
					return nil, fmt.Errorf("rule %d: invalid user agent glob pattern '%s': %w", i, pattern, err)
				}
				compiled.condition.userAgentGlobs = append(compiled.condition.userAgentGlobs, g)
			}
		}

		// Pre-parse IP/CIDR ranges
		if len(rule.Condition.IPs) > 0 {
			cidrs, err := iputil.ParseCIDRs(rule.Condition.IPs)
			if err != nil {
				return nil, fmt.Errorf("rule %d: invalid IP/CIDR patterns: %w", i, err)
			}
			compiled.condition.ipCIDRs = cidrs
		}

		compiledRules = append(compiledRules, compiled)
	}

	return &RuleProcessor{
		cfg:            cfg,
		trustedProxies: trustedProxies,
		compiledRules:  compiledRules,
	}, nil
}

// Process evaluates the configured rules against the request parameters according to the defined logic.
func (rp *RuleProcessor) Process(siteID, gtmID string, r *http.Request) LogProcessingResult {
	result := LogProcessingResult{
		ShouldInjectScripts:      false,
		ShouldLogToServer:        false, // Determined by the first final rule found, defaults to false
		ShouldLogScriptDownloads: false,
		AccumulatedScripts:       nil,
		AccumulatedAddLogData:    nil,
		TargetDestinations:       nil,
		AccumulatedJavaScriptOptions: struct {
			TrackURL       bool
			TrackTraceback bool
		}{},
	}
	accumulatedScriptsMap := make(map[string]config.ScriptInjectionSpec)
	accumulatedDataMap := make(map[string]config.AddLogDataSpec)

	clientIPString := iputil.GetClientIP(r, rp.trustedProxies, "")
	var clientIP net.IP
	if clientIPString != "" {
		clientIP = net.ParseIP(clientIPString)
	}
	userAgent := r.UserAgent()

	// Iterate through pre-compiled rules
	for i, compiled := range rp.compiledRules {
		currentRule := compiled.rule
		ruleID := i

		// Skip disabled rules entirely
		if !currentRule.Enabled {
			continue
		}

		if rp.matchCompiledCondition(ruleID, compiled.condition, siteID, gtmID, clientIP, userAgent, r) {
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

			// Accumulate JavaScript options (only overwrite if explicitly set in rule)
			if currentRule.JavaScriptOptions.TrackURL != (struct{ TrackURL bool }{TrackURL: false}).TrackURL {
				result.AccumulatedJavaScriptOptions.TrackURL = currentRule.JavaScriptOptions.TrackURL
			}
			if currentRule.JavaScriptOptions.TrackTraceback != (struct{ TrackTraceback bool }{TrackTraceback: false}).TrackTraceback {
				result.AccumulatedJavaScriptOptions.TrackTraceback = currentRule.JavaScriptOptions.TrackTraceback
			}

			// Check if this is a final rule (not continuing)
			if !currentRule.Continue {
				// This is a final rule - set logging decision and destinations
				result.ShouldLogToServer = true
				result.ShouldLogScriptDownloads = currentRule.LogScriptDownloads
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

// matchCompiledCondition checks if the request parameters match the pre-compiled condition.
// Uses pre-compiled glob patterns and CIDR ranges for better performance.
func (rp *RuleProcessor) matchCompiledCondition(ruleID int, cond compiledCondition, siteID, gtmID string, clientIP net.IP, userAgent string, r *http.Request) bool {
	// Check if condition is empty (matches everything)
	emptyCondition := cond.siteID == "" && len(cond.gtmIDs) == 0 && len(cond.userAgentGlobs) == 0 && len(cond.ipCIDRs) == 0 && len(cond.headers) == 0
	if emptyCondition {
		return true
	}

	// SiteID check
	if cond.siteID != "" {
		if cond.siteID != siteID {
			return false
		}
	}

	// GTMIDs check
	if len(cond.gtmIDs) > 0 {
		match := false
		for _, ruleGtmID := range cond.gtmIDs {
			if ruleGtmID == gtmID {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// Headers check
	if len(cond.headers) > 0 {
		if r == nil {
			return false
		}

		for headerName, headerCondition := range cond.headers {
			// Processing false value means the header must be absent
			if headerCondition == false {
				if r.Header.Get(headerName) != "" {
					return false
				}
				continue
			}

			// Value true means the header must exist (regardless of its value)
			if headerCondition == true {
				if r.Header.Get(headerName) == "" {
					return false
				}
				continue
			}

			// Check for specific header value
			expectedValue, ok := headerCondition.(string)
			if ok {
				actualValue := r.Header.Get(headerName)
				if actualValue != expectedValue {
					return false
				}
			} else {
				// Unsupported value type in condition
				return false
			}
		}
	}

	// UserAgents check - use pre-compiled glob patterns
	if len(cond.userAgentGlobs) > 0 {
		match := false
		for _, g := range cond.userAgentGlobs {
			if g.Match(userAgent) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	// IPs check - use pre-parsed CIDRs
	if len(cond.ipCIDRs) > 0 {
		if clientIP == nil {
			return false
		}
		if !iputil.IsIPInAnyCIDR(clientIP, cond.ipCIDRs) {
			return false
		}
	}

	// All conditions matched
	return true
}

// matchCondition checks if the request parameters match the rule's condition.
// DEPRECATED: Use matchCompiledCondition instead for better performance.
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
