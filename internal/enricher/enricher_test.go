package enricher_test // Changed package name to avoid import cycle

import (
	"bytes"
	"encoding/json" // Added import
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect" // Added import
	"testing"
	"time"

	// Added import
	"github.com/orgoj/weblogproxy/internal/config" // Added import
	"github.com/orgoj/weblogproxy/internal/enricher"
	// Added import
)

// Helper function to create a mock http.Request
func newMockRequest(method, path string, headers map[string]string, queryParams url.Values, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewBuffer(body))
	if queryParams != nil {
		req.URL.RawQuery = queryParams.Encode()
	}
	if headers != nil {
		for k, v := range headers {
			req.Header.Set(k, v)
		}
	}
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		if body != nil {
			req.Header.Set("Content-Type", "application/json") // Assume JSON body for POST tests
		}
	}
	return req
}

// Helper to create deep copy using JSON marshal/unmarshal to handle number types
func deepCopyMap(original map[string]interface{}) (map[string]interface{}, error) {
	if original == nil {
		return nil, nil
	}
	b, err := json.Marshal(original)
	if err != nil {
		return nil, err
	}
	var copied map[string]interface{}
	err = json.Unmarshal(b, &copied)
	if err != nil {
		return nil, err
	}
	return copied, nil
}

// Helper to add required Bunyan fields to expected output
func addBunyanFields(record map[string]interface{}) map[string]interface{} {
	if _, ok := record["v"]; !ok {
		record["v"] = 0
	}
	if _, ok := record["name"]; !ok {
		record["name"] = "weblogproxy"
	}
	if _, ok := record["hostname"]; !ok {
		record["hostname"] = "test-host"
	}
	if _, ok := record["pid"]; !ok {
		record["pid"] = float64(12345)
	}
	if _, ok := record["level"]; !ok {
		record["level"] = float64(enricher.DefaultLogLevel)
	}
	if _, ok := record["msg"]; !ok {
		record["msg"] = ""
	}
	// Add a dummy timestamp as ISO 8601 string
	if _, ok := record["time"]; !ok {
		record["time"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return record
}

// compareMapsIgnoreTime compares two records, but ignores the "time" field
func compareMapsIgnoreTime(t *testing.T, got, want map[string]interface{}) {
	// Check that the "time" field exists in the obtained result and has the correct format
	if timeValue, ok := got["time"]; ok {
		// Verify that the value is a string
		timeStr, ok := timeValue.(string)
		if !ok {
			t.Errorf("Expected 'time' to be a string, got %T", timeValue)
		} else {
			// Try to parse as RFC3339 or RFC3339Nano format
			_, err := time.Parse(time.RFC3339Nano, timeStr)
			if err != nil {
				// Try a less strict format
				_, err = time.Parse(time.RFC3339, timeStr)
				if err != nil {
					t.Errorf("Time '%s' is not in a valid ISO 8601 format: %v", timeStr, err)
				}
			}
		}
	} else {
		t.Error("Missing 'time' field in result")
	}

	// Create copies and remove the "time" field
	// Use json.Marshal/Unmarshal for deep copy
	gotBytes, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Failed to marshal 'got' map: %v", err)
		return
	}

	wantBytes, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("Failed to marshal 'want' map: %v", err)
		return
	}

	var gotCopy, wantCopy map[string]interface{}

	if err := json.Unmarshal(gotBytes, &gotCopy); err != nil {
		t.Fatalf("Failed to unmarshal 'got' map: %v", err)
		return
	}

	if err := json.Unmarshal(wantBytes, &wantCopy); err != nil {
		t.Fatalf("Failed to unmarshal 'want' map: %v", err)
		return
	}

	// Remove the "time" field
	delete(gotCopy, "time")
	delete(wantCopy, "time")

	// Compare remaining fields
	if !reflect.DeepEqual(gotCopy, wantCopy) {
		gotJSON, _ := json.MarshalIndent(gotCopy, "", "  ")
		wantJSON, _ := json.MarshalIndent(wantCopy, "", "  ")
		t.Errorf("Maps don't match (ignoring 'time'):\nGot: %s\nWant: %s", gotJSON, wantJSON)
	}
}

func TestEnrichAndMerge(t *testing.T) {
	// Prepare base hostname and pid for consistent results
	// Note: These might differ from the actual enricher package's init values,
	// but we control the baseRecord input in tests.
	testHostname := "test-host"
	testPid := 12345

	// Define test cases
	tests := []struct {
		name        string
		baseRecord  map[string]interface{}
		ruleAdds    []config.AddLogDataSpec
		destAdds    []config.AddLogDataSpec
		clientData  map[string]interface{}
		request     *http.Request
		want        map[string]interface{}
		wantErr     bool
		wantFinalKv map[string]interface{}
	}{
		{
			name: "Basic merge without enrichment",
			baseRecord: map[string]interface{}{
				"v":          float64(0),
				"name":       "weblogproxy",
				"hostname":   testHostname,
				"pid":        testPid,
				"level":      float64(enricher.DefaultLogLevel),
				"msg":        "",
				"site_id":    "site1",
				"client_ip":  "1.1.1.1",
				"user_agent": "TestAgent/1.0",
			},
			ruleAdds:   []config.AddLogDataSpec{},
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"msg": "hello world"},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"hello world"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":   testHostname,
				"pid":        float64(testPid),
				"level":      float64(enricher.DefaultLogLevel),
				"msg":        "hello world",
				"site_id":    "site1",
				"client_ip":  "1.1.1.1",
				"user_agent": "TestAgent/1.0",
			}),
			wantErr: false,
		},
		{
			name: "Static enrichment from rules",
			baseRecord: map[string]interface{}{
				"v":          float64(0),
				"name":       "weblogproxy",
				"hostname":   testHostname,
				"pid":        testPid,
				"level":      float64(enricher.DefaultLogLevel),
				"msg":        "",
				"site_id":    "site2",
				"client_ip":  "2.2.2.2",
				"user_agent": "TestAgent/2.0",
			},
			ruleAdds: []config.AddLogDataSpec{
				{Name: "rule_static", Source: "static", Value: "rule_value"},
				{Name: "common_field", Source: "static", Value: "from_rule"},
			},
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"data": "client stuff"},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"data":"client stuff"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":     testHostname,
				"pid":          float64(testPid),
				"level":        float64(enricher.DefaultLogLevel),
				"site_id":      "site2",
				"client_ip":    "2.2.2.2",
				"user_agent":   "TestAgent/2.0",
				"rule_static":  "rule_value",
				"common_field": "from_rule",
				"data":         "client stuff",
			}),
			wantErr: false,
		},
		{
			name: "Static enrichment from destination",
			baseRecord: map[string]interface{}{
				"hostname":   testHostname,
				"pid":        testPid,
				"level":      enricher.DefaultLogLevel,
				"site_id":    "site3",
				"client_ip":  "3.3.3.3",
				"user_agent": "TestAgent/3.0",
			},
			ruleAdds: []config.AddLogDataSpec{},
			destAdds: []config.AddLogDataSpec{
				{Name: "dest_static", Source: "static", Value: "dest_value"},
				{Name: "common_field", Source: "static", Value: "from_dest"},
			},
			clientData: map[string]interface{}{"payload": 123},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"payload":123}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":     testHostname,
				"pid":          float64(testPid),
				"level":        float64(enricher.DefaultLogLevel),
				"site_id":      "site3",
				"client_ip":    "3.3.3.3",
				"user_agent":   "TestAgent/3.0",
				"dest_static":  "dest_value",
				"common_field": "from_dest",
				"payload":      float64(123), // Client data
			}),
			wantErr: false,
		},
		{
			name: "Overwrite order: Client > Dest > Rule > Base",
			baseRecord: map[string]interface{}{
				"hostname":  testHostname,
				"pid":       testPid,
				"level":     enricher.DefaultLogLevel, // Base level
				"base_only": "base_val",
				"field1":    "from_base",
				"field2":    "from_base",
				"field3":    "from_base",
			},
			ruleAdds: []config.AddLogDataSpec{
				{Name: "rule_only", Source: "static", Value: "rule_val"},
				{Name: "field1", Source: "static", Value: "from_rule"}, // Overwrite base
				{Name: "field2", Source: "static", Value: "from_rule"},
				{Name: "field3", Source: "static", Value: "from_rule"},
			},
			destAdds: []config.AddLogDataSpec{
				{Name: "dest_only", Source: "static", Value: "dest_val"},
				{Name: "field2", Source: "static", Value: "from_dest"}, // Overwrite rule
				{Name: "field3", Source: "static", Value: "from_dest"},
			},
			clientData: map[string]interface{}{
				"client_only": "client_val",
				"field3":      "from_client", // Overwrite dest
				"level":       float64(50),   // Client overwrites level (Bunyan ERROR)
			},
			request: newMockRequest("POST", "/log", nil, nil, []byte(`{"client_only":"client_val", "field3":"from_client", "level": 50}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":    testHostname,
				"pid":         float64(testPid),
				"level":       float64(50), // Final: from client
				"base_only":   "base_val",
				"rule_only":   "rule_val",
				"dest_only":   "dest_val",
				"client_only": "client_val",
				"field1":      "from_rule",   // Final: from rule
				"field2":      "from_dest",   // Final: from dest
				"field3":      "from_client", // Final: from client
			}),
			wantErr: false,
		},
		{
			name: "Enrichment from header",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site4",
			},
			ruleAdds:   []config.AddLogDataSpec{{Name: "x_req_id", Source: "header", Value: "X-Request-ID"}},
			destAdds:   []config.AddLogDataSpec{{Name: "user_ctry", Source: "header", Value: "X-User-Country"}},
			clientData: map[string]interface{}{"msg": "header test"},
			request: newMockRequest("POST", "/log", map[string]string{
				"X-Request-ID":   "req-abc",
				"X-User-Country": "CZ",
				"Irrelevant":     "data",
			}, nil, []byte(`{"msg":"header test"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":  testHostname,
				"pid":       float64(testPid),
				"level":     float64(enricher.DefaultLogLevel),
				"site_id":   "site4",
				"x_req_id":  "req-abc",     // From ruleAdds
				"user_ctry": "CZ",          // From destAdds
				"msg":       "header test", // From clientData
			}),
			wantErr: false,
		},
		{
			name: "Enrichment from query parameters",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site5",
			},
			ruleAdds:   []config.AddLogDataSpec{{Name: "utm_source", Source: "query", Value: "utm_source"}},
			destAdds:   []config.AddLogDataSpec{{Name: "campaign", Source: "query", Value: "utm_campaign"}},
			clientData: map[string]interface{}{"msg": "query test"},
			request: newMockRequest("POST", "/log", nil, url.Values{
				"utm_source":   {"google"},
				"utm_campaign": {"summer_sale"},
				"other":        {"ignore"},
			}, []byte(`{"msg":"query test"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":   testHostname,
				"pid":        float64(testPid),
				"level":      float64(enricher.DefaultLogLevel),
				"site_id":    "site5",
				"utm_source": "google",      // From ruleAdds
				"campaign":   "summer_sale", // From destAdds
				"msg":        "query test",  // From clientData
			}),
			wantErr: false,
		},
		{
			name: "Enrichment from post body (clientData) - simple field",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site6",
			},
			ruleAdds: []config.AddLogDataSpec{{Name: "event_name", Source: "post", Value: "event"}},
			destAdds: []config.AddLogDataSpec{{Name: "product_sku", Source: "post", Value: "details.sku"}}, // Nested
			clientData: map[string]interface{}{
				"msg":   "post test",
				"event": "purchase",
				"details": map[string]interface{}{
					"sku":   "ABC-123",
					"price": 99.99,
				},
			},
			request: newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"post test", "event":"purchase", "details":{"sku":"ABC-123", "price":99.99}}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":    testHostname,
				"pid":         float64(testPid),
				"level":       float64(enricher.DefaultLogLevel),
				"site_id":     "site6",
				"event_name":  "purchase",                                               // From ruleAdds + clientData.event
				"product_sku": "ABC-123",                                                // From destAdds + clientData.details.sku
				"msg":         "post test",                                              // Original client data
				"event":       "purchase",                                               // Original client data
				"details":     map[string]interface{}{"sku": "ABC-123", "price": 99.99}, // Original client data
			}),
			wantErr: false,
		},
		{
			name: "Enrichment from post body (clientData) - missing nested field",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site6",
			},
			ruleAdds:   []config.AddLogDataSpec{{Name: "should_not_exist", Source: "post", Value: "details.nonexistent.field"}},
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"msg": "post missing field test", "details": map[string]interface{}{"sku": "XYZ-789"}},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"post missing field test", "details":{"sku":"XYZ-789"}}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname": testHostname,
				"pid":      float64(testPid),
				"level":    float64(enricher.DefaultLogLevel),
				"site_id":  "site6",
				"msg":      "post missing field test",                // Original client data
				"details":  map[string]interface{}{"sku": "XYZ-789"}, // Original client data
				// "should_not_exist" should not be present
			}),
			wantErr: false,
		},
		{
			name: "Overwrite base level via static rule",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site7",
			},
			ruleAdds:   []config.AddLogDataSpec{{Name: "level", Source: "static", Value: "40"}}, // Bunyan WARN
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"msg": "level change test"},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"level change test"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname": testHostname,
				"pid":      float64(testPid),
				"level":    float64(40), // Overwritten by rule
				"site_id":  "site7",
				"msg":      "level change test",
			}),
			wantErr: false,
		},
		{
			name: "Error case: Invalid static level value",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site8",
			},
			ruleAdds:   []config.AddLogDataSpec{{Name: "level", Source: "static", Value: "not-a-number"}},
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"msg": "error test"},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"error test"}`)),
			want:       nil, // Expecting error, final map doesn't matter
			wantErr:    true,
		},
		{
			name: "Error case: Invalid level type (not string)",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site9",
			},
			// Simulate a scenario where somehow a non-string static value is provided (config validation should prevent this)
			// For test purposes, we create the spec directly.
			ruleAdds: []config.AddLogDataSpec{{Name: "level", Source: "static", Value: "123"}}, // Valid number string
			// Modify the value after creation to bypass normal loading/validation if it existed
			// This test is more about mergeAddLogData's internal handling if type assumption fails
			destAdds: []config.AddLogDataSpec{{Name: "level", Source: "static", Value: "abc"}}, // Invalid string is handled above
			// Let's try adding one with a non-string Value, though config loading prevents this
			// We'll use a query source that returns a number-like string but test internal func
			// Need a test that exercises the type assertion `ok := value.(string)` failing inside mergeAddLogData
			// We'll test this by setting a non-string value in clientData and using source: "post"
			clientData: map[string]interface{}{"level_val": 40.5}, // Provide a float directly
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"level_val": 40.5}`)),
			want:       nil,
			wantErr:    true, // Expect error because "post" source gave float64, not string for level conversion
		},
		{
			name: "Missing header/query/post values are skipped",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site10",
			},
			ruleAdds: []config.AddLogDataSpec{
				{Name: "missing_header", Source: "header", Value: "X-Non-Existent"},
				{Name: "present_static", Source: "static", Value: "exists"},
			},
			destAdds: []config.AddLogDataSpec{
				{Name: "missing_query", Source: "query", Value: "non_existent_param"},
				{Name: "missing_post", Source: "post", Value: "non_existent_field"},
			},
			clientData: map[string]interface{}{"msg": "missing values test"},
			request:    newMockRequest("POST", "/log", map[string]string{"X-Real": "Header"}, url.Values{"real_param": {"value"}}, []byte(`{"msg":"missing values test"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":       testHostname,
				"pid":            float64(testPid),
				"level":          float64(enricher.DefaultLogLevel),
				"site_id":        "site10",
				"present_static": "exists",              // Should be present
				"msg":            "missing values test", // From clientData
				// missing_header, missing_query, missing_post should NOT be present
			}),
			wantErr: false,
		},
		{
			name: "Multiple rules and destinations merging",
			baseRecord: map[string]interface{}{
				"hostname": testHostname, "pid": testPid, "level": enricher.DefaultLogLevel, "site_id": "site11",
			},
			ruleAdds: []config.AddLogDataSpec{
				{Name: "field_a", Source: "static", Value: "rule1_a"},
				{Name: "field_b", Source: "static", Value: "rule1_b"}, // This will be overwritten by the next entry
				{Name: "field_c", Source: "static", Value: "rule1_c"}, // This will be overwritten by destAdds
				{Name: "field_b", Source: "static", Value: "rule2_b"}, // This should be the final value for field_b from rules
			},
			destAdds: []config.AddLogDataSpec{
				{Name: "field_c", Source: "static", Value: "dest1_c"}, // Overwrites rule1_c
				{Name: "field_d", Source: "static", Value: "dest1_d"},
			},
			clientData: map[string]interface{}{
				"msg":     "multi merge",
				"field_d": "client_d", // Overwrites dest1_d
			},
			request: newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"multi merge", "field_d":"client_d"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname": testHostname,
				"pid":      float64(testPid),
				"level":    float64(enricher.DefaultLogLevel),
				"site_id":  "site11",
				"field_a":  "rule1_a",     // From rule1
				"field_b":  "rule2_b",     // From rule2 (overwrote rule1)
				"field_c":  "dest1_c",     // From dest1 (overwrote rule1)
				"field_d":  "client_d",    // From client (overwrote dest1)
				"msg":      "multi merge", // From client
			}),
			wantErr: false,
		},
		{
			name: "Remove field with false value",
			baseRecord: map[string]interface{}{
				"hostname":        testHostname,
				"pid":             testPid,
				"level":           enricher.DefaultLogLevel,
				"site_id":         "site11",
				"field_to_keep":   "keep_me",
				"field_to_remove": "initial_value",
			},
			ruleAdds: []config.AddLogDataSpec{
				// Removal instruction
				{Name: "field_to_remove", Source: "static", Value: "false"},
				// Normal addition
				{Name: "new_field", Source: "static", Value: "new_value"},
			},
			destAdds:   []config.AddLogDataSpec{},
			clientData: map[string]interface{}{"msg": "field removal test"},
			request:    newMockRequest("POST", "/log", nil, nil, []byte(`{"msg":"field removal test"}`)),
			want: addBunyanFields(map[string]interface{}{
				"hostname":      testHostname,
				"pid":           float64(testPid),
				"level":         float64(enricher.DefaultLogLevel),
				"site_id":       "site11",
				"field_to_keep": "keep_me",
				"new_field":     "new_value",
				"msg":           "field removal test",
				// field_to_remove should not be in the expected output
			}),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := enricher.EnrichAndMerge(tt.baseRecord, tt.ruleAdds, tt.destAdds, tt.clientData, tt.request)
			if (err != nil) != tt.wantErr {
				t.Errorf("EnrichAndMerge() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}

			// Use the new comparison function
			compareMapsIgnoreTime(t, got, tt.want)
		})
	}
}

// Additional test functions can be added here

func TestCreateBaseRecord(t *testing.T) {
	hostname, _ := os.Hostname()
	pid := os.Getpid()

	tests := []struct {
		name      string
		siteID    string
		gtmID     string
		clientIP  string
		userAgent string
		want      map[string]interface{}
	}{
		{
			name:      "With GTM ID",
			siteID:    "siteA",
			gtmID:     "GTM-XYZ",
			clientIP:  "10.0.0.1",
			userAgent: "AgentA",
			want: map[string]interface{}{
				"v":          float64(0),
				"name":       "weblogproxy",
				"hostname":   hostname,
				"pid":        pid,
				"level":      enricher.DefaultLogLevel,
				"msg":        "",
				"site_id":    "siteA",
				"gtm_id":     "GTM-XYZ",
				"client_ip":  "10.0.0.1",
				"user_agent": "AgentA",
			},
		},
		{
			name:      "Without GTM ID",
			siteID:    "siteB",
			gtmID:     "", // Empty GTM ID
			clientIP:  "10.0.0.2",
			userAgent: "AgentB",
			want: map[string]interface{}{
				"v":        float64(0),
				"name":     "weblogproxy",
				"hostname": hostname,
				"pid":      pid,
				"level":    enricher.DefaultLogLevel,
				"msg":      "",
				"site_id":  "siteB",
				// gtm_id should be absent
				"client_ip":  "10.0.0.2",
				"user_agent": "AgentB",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enricher.CreateBaseRecord(tt.siteID, tt.gtmID, tt.clientIP, tt.userAgent)

			// Need to handle potential type difference for pid (int vs float64)
			// Convert want pid to float64 for comparison if necessary
			if pidVal, ok := tt.want["pid"].(int); ok {
				tt.want["pid"] = float64(pidVal)
			}
			if pidVal, ok := tt.want["level"].(int); ok {
				tt.want["level"] = float64(pidVal)
			}

			// Use deepCopyMap on both for consistent type handling (esp. numbers)
			gotCopy, _ := deepCopyMap(got)
			wantCopy, _ := deepCopyMap(tt.want)

			if !reflect.DeepEqual(gotCopy, wantCopy) {
				gotJSON, _ := json.MarshalIndent(gotCopy, "", "  ")
				wantJSON, _ := json.MarshalIndent(wantCopy, "", "  ")
				t.Errorf("CreateBaseRecord() = %s, want %s", string(gotJSON), string(wantJSON))
			}
		})
	}
}
