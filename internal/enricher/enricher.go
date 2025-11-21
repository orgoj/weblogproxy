// internal/enricher/enricher.go

package enricher

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/orgoj/weblogproxy/internal/config"
)

// Standard Bunyan fields
const (
	fieldVersion  = "v"
	fieldName     = "name"
	fieldLevel    = "level"
	fieldTime     = "time"
	fieldMsg      = "msg"
	fieldHostname = "hostname"
	fieldPid      = "pid"
)

// Log levels according to Bunyan
const (
	TRACE = 10
	DEBUG = 20
	INFO  = 30
	WARN  = 40
	ERROR = 50
	FATAL = 60
)

// DefaultLogLevel is INFO (30)
const DefaultLogLevel = INFO

// Cached system values to avoid repeated syscalls
var (
	cachedHostname string
	cachedPid      int
	cacheOnce      sync.Once
)

// initCachedValues initializes cached system values once
func initCachedValues() {
	hostname, err := os.Hostname()
	if err != nil {
		cachedHostname = "unknown"
	} else {
		cachedHostname = hostname
	}
	cachedPid = os.Getpid()
}

// CreateBaseRecord creates a new log record with required Bunyan fields
// Uses cached hostname and PID to avoid repeated system calls
func CreateBaseRecord(siteID, gtmID, clientIP string) map[string]interface{} {
	// Initialize cached values once
	cacheOnce.Do(initCachedValues)

	record := map[string]interface{}{
		fieldVersion:  0, // Integer
		fieldName:     "weblogproxy",
		fieldHostname: cachedHostname,  // Cached value
		fieldPid:      cachedPid,       // Cached value
		fieldLevel:    DefaultLogLevel, // Integer
		fieldMsg:      "",
		"site_id":     siteID,
		"client_ip":   clientIP,
	}
	if gtmID != "" {
		record["gtm_id"] = gtmID
	}
	return record
}

// EnrichAndMerge combines data from multiple sources into a single log record
func EnrichAndMerge(baseRecord map[string]interface{}, ruleAdds []config.AddLogDataSpec, destAdds []config.AddLogDataSpec, clientData map[string]interface{}, request *http.Request) (map[string]interface{}, error) {
	// Make a copy of the base record to avoid modifying the original
	record := make(map[string]interface{}, len(baseRecord))
	for k, v := range baseRecord {
		record[k] = v
	}

	// Process rule additions
	for _, add := range ruleAdds {
		if err := processAddLogData(add, record, request, clientData); err != nil {
			return nil, fmt.Errorf("processing rule add: %w", err)
		}
	}

	// Process destination additions
	for _, add := range destAdds {
		if err := processAddLogData(add, record, request, clientData); err != nil {
			return nil, fmt.Errorf("processing destination add: %w", err)
		}
	}

	// Merge client data last (highest precedence)
	if clientData != nil {
		// Explicitly handle 'message' field to map it to 'msg'
		if msgValue, msgExists := clientData["message"]; msgExists {
			if msgStr, ok := msgValue.(string); ok {
				record[fieldMsg] = msgStr
			} else {
				// If message is not a string, maybe log a warning or convert?
				// For now, let's just assign it if it's not nil
				if msgValue != nil {
					// Or should we assign the string representation?
					record[fieldMsg] = fmt.Sprintf("%v", msgValue)
				}
			}
		}

		for k, v := range clientData {
			// Skip 'message' field as it was already handled
			if k == "message" {
				continue
			}
			record[k] = v
		}
	}

	// Add timestamp last to ensure it's not overwritten
	// Using ISO 8601 format in UTC according to Bunyan specification
	record[fieldTime] = time.Now().UTC().Format(time.RFC3339Nano)

	// Ensure all required Bunyan fields are present
	ensureBunyanFields(record)

	return record, nil
}

// processAddLogData applies a single AddLogDataSpec to the record
func processAddLogData(add config.AddLogDataSpec, record map[string]interface{}, request *http.Request, clientData map[string]interface{}) error {
	var value interface{}
	var found bool

	// Special case: removing a field if add.Source is "static" and add.Value is "false"
	if add.Source == "static" && add.Value == "false" {
		delete(record, add.Name)
		return nil
	}

	switch add.Source {
	case "static":
		value = add.Value
		found = true
	case "header":
		if request != nil {
			value = request.Header.Get(add.Value)
			found = value != ""
		}
	case "query":
		if request != nil {
			value = request.URL.Query().Get(add.Value)
			found = value != ""
		}
	case "post":
		if request != nil && request.Body != nil {
			value, found = getValueFromMap(clientData, add.Value)
		}
	default:
		return fmt.Errorf("unknown source type: %s", add.Source)
	}

	if !found {
		return nil
	}

	// Special handling for level field
	if add.Name == fieldLevel {
		if strValue, ok := value.(string); ok {
			levelVal, err := strconv.ParseFloat(strValue, 64)
			if err != nil {
				return fmt.Errorf("invalid numeric value for level field '%s': %w", strValue, err)
			}
			record[add.Name] = levelVal
		} else if numValue, ok := value.(float64); ok {
			record[add.Name] = numValue
		} else {
			return fmt.Errorf("level field must be a string or number, got %T", value)
		}
	} else {
		record[add.Name] = value
	}

	return nil
}

// getValueFromMap retrieves a value from a nested map using dot notation
func getValueFromMap(data map[string]interface{}, key string) (interface{}, bool) {
	if data == nil {
		return nil, false
	}

	parts := strings.Split(key, ".")
	var current interface{} = data

	for i, part := range parts {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return nil, false
		}

		value, exists := currentMap[part]
		if !exists {
			return nil, false
		}

		if i == len(parts)-1 {
			return value, true
		}

		current = value
	}

	return nil, false
}

// ensureBunyanFields ensures all required Bunyan fields are present with correct types
func ensureBunyanFields(record map[string]interface{}) {
	// Version - must be integer
	if _, ok := record[fieldVersion]; !ok {
		record[fieldVersion] = 0
	} else if v, ok := record[fieldVersion].(float64); ok {
		record[fieldVersion] = int(v)
	}

	// Name - must be string
	if _, ok := record[fieldName]; !ok {
		record[fieldName] = "weblogproxy"
	}

	// Level - must be integer
	if _, ok := record[fieldLevel]; !ok {
		record[fieldLevel] = DefaultLogLevel
	} else if v, ok := record[fieldLevel].(float64); ok {
		record[fieldLevel] = int(v)
	} else if v, ok := record[fieldLevel].(string); ok {
		if levelVal, err := strconv.Atoi(v); err == nil {
			record[fieldLevel] = levelVal
		} else {
			record[fieldLevel] = DefaultLogLevel
		}
	}

	// Hostname - must be string
	if _, ok := record[fieldHostname]; !ok {
		hostname, _ := os.Hostname()
		record[fieldHostname] = hostname
	}

	// PID - must be integer
	if _, ok := record[fieldPid]; !ok {
		record[fieldPid] = os.Getpid()
	} else if v, ok := record[fieldPid].(float64); ok {
		record[fieldPid] = int(v)
	}

	// Message - must be string
	if _, ok := record[fieldMsg]; !ok {
		record[fieldMsg] = ""
	}

	// Time - must be ISO 8601 string
	if _, ok := record[fieldTime]; !ok {
		record[fieldTime] = time.Now().UTC().Format(time.RFC3339Nano)
	}
}
