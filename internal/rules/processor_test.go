package rules

import (
	"net/http"
	"reflect"
	"sort"
	"testing"

	"github.com/orgoj/weblogproxy/internal/config"
)

type testCase struct {
	name               string
	logConfig          []config.LogRule
	trustedProxies     []string
	siteID             string
	gtmID              string
	clientIP           string
	userAgent          string
	expectedResult     LogProcessingResult
	expectError        bool
	forwardedForHeader string // Simulate X-Forwarded-For
}

func TestRuleProcessor_Process(t *testing.T) {
	tests := []testCase{
		{
			name:           "NoRules",
			logConfig:      []config.LogRule{},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: false, ShouldInjectScripts: false, TargetDestinations: []string{}},
		},
		{
			name: "SimpleMatch_SiteID",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true, LogDestinations: []string{"file"}},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"file"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "SimpleMatch_IP",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{IPs: []string{"1.1.1.1"}}, Enabled: true, LogDestinations: []string{"ip_dest"}},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"ip_dest"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "SimpleMatch_IP_CIDR",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{IPs: []string{"1.2.0.0/16"}}, Enabled: true, LogDestinations: []string{"cidr_dest"}},
			},
			siteID:         "test",
			clientIP:       "1.2.3.4",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"cidr_dest"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name:           "SimpleMatch_IP_XFF_Trusted",
			trustedProxies: []string{"10.0.0.1"},
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{IPs: []string{"192.168.1.100"}}, Enabled: true, LogDestinations: []string{"xff_dest"}},
			},
			siteID:             "test",
			clientIP:           "10.0.0.1",
			forwardedForHeader: "192.168.1.100, 10.0.0.1",
			userAgent:          "TestAgent",
			expectedResult:     LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"xff_dest"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name:           "NoMatch_IP_XFF_Untrusted",
			trustedProxies: []string{"10.0.0.2"},
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{IPs: []string{"192.168.1.100"}}, Enabled: true},
			},
			siteID:             "test",
			clientIP:           "10.0.0.1",
			forwardedForHeader: "192.168.1.100",
			userAgent:          "TestAgent",
			expectedResult:     LogProcessingResult{ShouldLogToServer: false, ShouldInjectScripts: false, TargetDestinations: []string{}},
		},
		{
			name: "SimpleMatch_UserAgent",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{UserAgents: []string{"*Bot*"}}, Enabled: true, LogDestinations: []string{"ua_dest"}},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "GoogleBot/2.1",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"ua_dest"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "NoMatch",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "other"}, Enabled: true},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: false, ShouldInjectScripts: false, TargetDestinations: []string{}},
		},
		{
			name: "Continue",
			logConfig: []config.LogRule{
				{
					Condition:       config.LogRuleCondition{SiteID: "test"},
					Enabled:         true,
					Continue:        true,
					AddLogData:      []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}},
					ScriptInjection: []config.ScriptInjectionSpec{{URL: "/script1.js"}},
					LogDestinations: []string{"rule1_dest"},
				},
				{
					Condition:       config.LogRuleCondition{UserAgents: []string{"*Agent*"}},
					Enabled:         true,
					AddLogData:      []config.AddLogDataSpec{{Name: "rule2", Source: "static", Value: "value2"}},
					ScriptInjection: []config.ScriptInjectionSpec{{URL: "/script2.js"}},
					LogDestinations: []string{"final_dest"},
				},
			},
			siteID:    "test",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:     true,
				ShouldInjectScripts:   true,
				TargetDestinations:    []string{"final_dest"},
				AccumulatedAddLogData: []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}, {Name: "rule2", Source: "static", Value: "value2"}},
				AccumulatedScripts:    []config.ScriptInjectionSpec{{URL: "/script1.js"}, {URL: "/script2.js"}},
			},
		},
		{
			name: "Continue_LastRuleContinue",
			logConfig: []config.LogRule{
				{
					Condition:       config.LogRuleCondition{SiteID: "test"},
					Enabled:         true,
					Continue:        true,
					AddLogData:      []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}},
					LogDestinations: []string{"rule1_dest"},
				},
				{
					Condition:  config.LogRuleCondition{UserAgents: []string{"*Agent*"}},
					Enabled:    true,
					Continue:   true,
					AddLogData: []config.AddLogDataSpec{{Name: "rule2", Source: "static", Value: "value2"}},
				},
			},
			siteID:    "test",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:     false,
				ShouldInjectScripts:   true,
				TargetDestinations:    []string{},
				AccumulatedAddLogData: []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}, {Name: "rule2", Source: "static", Value: "value2"}},
				AccumulatedScripts:    []config.ScriptInjectionSpec{},
			},
		},
		{
			name: "StopOnMatch",
			logConfig: []config.LogRule{
				{
					Condition:       config.LogRuleCondition{SiteID: "test"},
					Enabled:         true,
					Continue:        false,
					AddLogData:      []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}},
					ScriptInjection: []config.ScriptInjectionSpec{{URL: "/script1.js"}},
					LogDestinations: []string{"rule1_dest"},
				},
				{
					Condition:       config.LogRuleCondition{UserAgents: []string{"*Agent*"}},
					Enabled:         true,
					LogDestinations: []string{"ignored_dest"},
				},
			},
			siteID:    "test",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:     true,
				ShouldInjectScripts:   true,
				TargetDestinations:    []string{"rule1_dest"},
				AccumulatedAddLogData: []config.AddLogDataSpec{{Name: "rule1", Source: "static", Value: "value1"}},
				AccumulatedScripts:    []config.ScriptInjectionSpec{{URL: "/script1.js"}},
			},
		},
		{
			name: "DataAccumulation_Overwrite",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{}, Enabled: true, Continue: true, AddLogData: []config.AddLogDataSpec{{Name: "keyA", Source: "static", Value: "valueA1"}, {Name: "keyB", Source: "static", Value: "valueB1"}}},
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true, Continue: true, AddLogData: []config.AddLogDataSpec{{Name: "keyA", Source: "static", Value: "valueA2"}, {Name: "keyC", Source: "static", Value: "valueC"}}},
				{Condition: config.LogRuleCondition{UserAgents: []string{"*"}}, Enabled: true, LogDestinations: []string{"data_dest"}},
			},
			siteID:    "test",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:     true,
				ShouldInjectScripts:   true,
				TargetDestinations:    []string{"data_dest"},
				AccumulatedAddLogData: []config.AddLogDataSpec{{Name: "keyA", Source: "static", Value: "valueA2"}, {Name: "keyB", Source: "static", Value: "valueB1"}, {Name: "keyC", Source: "static", Value: "valueC"}},
				AccumulatedScripts:    []config.ScriptInjectionSpec{},
			},
		},
		{
			name: "ScriptAccumulation_Deduplicate",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{}, Enabled: true, Continue: true, ScriptInjection: []config.ScriptInjectionSpec{{URL: "/common.js"}, {URL: "/script1.js", Async: true}}},
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true, Continue: true, ScriptInjection: []config.ScriptInjectionSpec{{URL: "/script2.js", Defer: true}, {URL: "/common.js"}}},
				{Condition: config.LogRuleCondition{UserAgents: []string{"*"}}, Enabled: true, LogDestinations: []string{"script_dest"}},
			},
			siteID:    "test",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:     true,
				ShouldInjectScripts:   true,
				TargetDestinations:    []string{"script_dest"},
				AccumulatedAddLogData: []config.AddLogDataSpec{},
				AccumulatedScripts:    []config.ScriptInjectionSpec{{URL: "/common.js"}, {URL: "/script1.js", Async: true}, {URL: "/script2.js", Defer: true}},
			},
		},
		{
			name: "DestinationOverride_Continue",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true, Continue: true, LogDestinations: []string{"rule1_dest"}},
				{Condition: config.LogRuleCondition{UserAgents: []string{"*"}}, Enabled: true, LogDestinations: []string{"rule2_dest"}},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"rule2_dest"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "DestinationOverride_Empty",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true, Continue: true, LogDestinations: []string{"rule1_dest"}},
				{Condition: config.LogRuleCondition{UserAgents: []string{"*"}}, Enabled: true, LogDestinations: nil},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: nil, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "DefaultDestinations",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: true},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: nil, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "DisabledRule",
			logConfig: []config.LogRule{
				{Condition: config.LogRuleCondition{SiteID: "test"}, Enabled: false},
				{Condition: config.LogRuleCondition{}, Enabled: true, LogDestinations: []string{"default"}},
			},
			siteID:         "test",
			clientIP:       "1.1.1.1",
			userAgent:      "TestAgent",
			expectedResult: LogProcessingResult{ShouldLogToServer: true, ShouldInjectScripts: true, TargetDestinations: []string{"default"}, AccumulatedAddLogData: []config.AddLogDataSpec{}, AccumulatedScripts: []config.ScriptInjectionSpec{}},
		},
		{
			name: "HeaderCondition_Match",
			logConfig: []config.LogRule{
				{
					Condition: config.LogRuleCondition{
						Headers: map[string]interface{}{
							"X-Test-Header": "TestValue",
							"User-Agent":    true, // Just header existence
						},
					},
					Enabled:         true,
					LogDestinations: []string{"header_match_dest"},
				},
			},
			siteID:    "test-header",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:   true,
				ShouldInjectScripts: true,
				TargetDestinations:  []string{"header_match_dest"},
			},
		},
		{
			name: "HeaderCondition_NoMatch",
			logConfig: []config.LogRule{
				{
					Condition: config.LogRuleCondition{
						Headers: map[string]interface{}{
							"X-Test-Header": "WrongValue",
						},
					},
					Enabled:         true,
					LogDestinations: []string{"header_nomatch_dest"},
				},
			},
			siteID:    "test-header",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:   false,
				ShouldInjectScripts: false,
				TargetDestinations:  []string{},
			},
		},
		{
			name: "HeaderCondition_FalseValue",
			logConfig: []config.LogRule{
				{
					Condition: config.LogRuleCondition{
						Headers: map[string]interface{}{
							"X-Unexpected-Header": false, // Header must not exist
						},
					},
					Enabled:         true,
					LogDestinations: []string{"header_false_dest"},
				},
			},
			siteID:    "test-header",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:   true,
				ShouldInjectScripts: true,
				TargetDestinations:  []string{"header_false_dest"},
			},
		},
		{
			name: "AddLogData_Remove",
			logConfig: []config.LogRule{
				{
					Condition:  config.LogRuleCondition{SiteID: "test-remove"},
					Enabled:    true,
					Continue:   true,
					AddLogData: []config.AddLogDataSpec{{Name: "field1", Source: "static", Value: "initial"}},
				},
				{
					Condition:       config.LogRuleCondition{},
					Enabled:         true,
					AddLogData:      []config.AddLogDataSpec{{Name: "field1", Source: "static", Value: "false"}}, // Field removal
					LogDestinations: []string{"remove_field_dest"},
				},
			},
			siteID:    "test-remove",
			clientIP:  "1.1.1.1",
			userAgent: "TestAgent",
			expectedResult: LogProcessingResult{
				ShouldLogToServer:   true,
				ShouldInjectScripts: true,
				TargetDestinations:  []string{"remove_field_dest"},
				// Processor only collects data, removal is done by the enricher
				AccumulatedAddLogData: []config.AddLogDataSpec{{Name: "field1", Source: "static", Value: "false"}},
			},
		},
		// TODO: Add tests for GTM ID matching
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := NewRuleProcessor(&config.Config{
				LogConfig: tt.logConfig,
				Server: struct {
					Host             string   `yaml:"host"`
					Port             int      `yaml:"port"`
					Mode             string   `yaml:"mode"`
					Protocol         string   `yaml:"protocol"`
					Domain           string   `yaml:"domain"`
					PathPrefix       string   `yaml:"path_prefix"`
					TrustedProxies   []string `yaml:"trusted_proxies"`
					HealthAllowedIPs []string `yaml:"health_allowed_ips"`
					CORS             struct {
						Enabled        bool     `yaml:"enabled"`
						AllowedOrigins []string `yaml:"allowed_origins"`
						MaxAge         int      `yaml:"max_age"`
					} `yaml:"cors"`
					Headers       map[string]string `yaml:"headers"`
					RequestLimits struct {
						MaxBodySize int `yaml:"max_body_size"`
						RateLimit   int `yaml:"rate_limit"`
					} `yaml:"request_limits"`
					JavaScript struct {
						GlobalObjectName string `yaml:"global_object_name"`
					} `yaml:"javascript"`
					UnknownRoute struct {
						Code         int    `yaml:"code"`
						CacheControl string `yaml:"cache_control"`
					} `yaml:"unknown_route"`
					ClientIPHeader string `yaml:"client_ip_header"`
				}{
					TrustedProxies: tt.trustedProxies,
					Mode:           "embedded",
					Protocol:       "http",
					UnknownRoute: struct {
						Code         int    `yaml:"code"`
						CacheControl string `yaml:"cache_control"`
					}{Code: 200, CacheControl: "public, max-age=3600"},
					ClientIPHeader: "",
				},
			})
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return // Don't proceed if error was expected
			}
			if err != nil {
				t.Fatalf("NewRuleProcessor() error = %v", err)
			}

			// Create request with appropriate headers
			req := &http.Request{
				RemoteAddr: tt.clientIP,
				Header:     make(http.Header),
			}
			req.Header.Set("User-Agent", tt.userAgent)
			if tt.forwardedForHeader != "" {
				req.Header.Set("X-Forwarded-For", tt.forwardedForHeader)
			}

			// Adding headers for header condition tests
			if tt.name == "HeaderCondition_Match" {
				req.Header.Set("X-Test-Header", "TestValue")
			}

			result := p.Process(tt.siteID, tt.gtmID, req)

			// Sort slices for comparison
			sort.Strings(result.TargetDestinations)
			sort.Strings(tt.expectedResult.TargetDestinations)
			sortAddLogDataSpecs(result.AccumulatedAddLogData)
			sortAddLogDataSpecs(tt.expectedResult.AccumulatedAddLogData)
			sortScriptInjectionSpecs(result.AccumulatedScripts)
			sortScriptInjectionSpecs(tt.expectedResult.AccumulatedScripts)

			// --- Normalize nil slices to empty slices for DeepEqual ---
			if result.TargetDestinations == nil {
				result.TargetDestinations = []string{}
			}
			if tt.expectedResult.TargetDestinations == nil {
				tt.expectedResult.TargetDestinations = []string{}
			}
			if result.AccumulatedAddLogData == nil {
				result.AccumulatedAddLogData = []config.AddLogDataSpec{}
			}
			if tt.expectedResult.AccumulatedAddLogData == nil {
				tt.expectedResult.AccumulatedAddLogData = []config.AddLogDataSpec{}
			}
			if result.AccumulatedScripts == nil {
				result.AccumulatedScripts = []config.ScriptInjectionSpec{}
			}
			if tt.expectedResult.AccumulatedScripts == nil {
				tt.expectedResult.AccumulatedScripts = []config.ScriptInjectionSpec{}
			}
			// --- End Normalization ---

			if !reflect.DeepEqual(result, tt.expectedResult) {
				t.Errorf("Process() got = %+v, \nwant %+v", result, tt.expectedResult)
			}
		})
	}
}

// Helper functions for sorting slices in ProcessingResult for stable comparison

func sortAddLogDataSpecs(specs []config.AddLogDataSpec) {
	sort.Slice(specs, func(i, j int) bool {
		if specs[i].Name != specs[j].Name {
			return specs[i].Name < specs[j].Name
		}
		if specs[i].Source != specs[j].Source {
			return specs[i].Source < specs[j].Source
		}
		return specs[i].Value < specs[j].Value
	})
}

func sortScriptInjectionSpecs(specs []config.ScriptInjectionSpec) {
	sort.Slice(specs, func(i, j int) bool {
		return specs[i].URL < specs[j].URL
	})
}
