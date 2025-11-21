package truncate

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"strings"
)

const (
	ellipsis              = "..."
	minTruncateLength     = 10    // Minimum length a string must have after truncation (excluding ellipsis)
	debugTruncation       = false // Enable debug logging for truncation process
	maxTruncateIterations = 100   // Safety limit for truncation loop
	maxNestedDepth        = 10    // Maximum depth to search for truncatable nested structures
)

// TruncateMapIfNeeded checks if the JSON representation size of the data map exceeds the limit.
// If it does, it iteratively finds the longest string value within the map (including nested maps and slices)
// and truncates it until the total size is within the limit or no more strings can be truncated.
// It modifies the map in place.
// Returns true if any truncation occurred, false otherwise, and an error if marshalling fails.
func TruncateMapIfNeeded(data *map[string]interface{}, limit int64) (bool, error) {
	if data == nil {
		return false, fmt.Errorf("input data map cannot be nil")
	}
	if limit <= 0 {
		return false, fmt.Errorf("limit must be positive")
	}

	truncated := false
	originalSize, err := estimateSize(*data)
	if err != nil {
		return false, fmt.Errorf("failed to estimate initial size: %w", err)
	}

	currentSize := originalSize
	if debugTruncation {
		log.Printf("[DEBUG] Initial data size %d, limit %d", currentSize, limit)
	}

	iterations := 0
	for currentSize > limit && iterations < maxTruncateIterations {
		iterations++
		if debugTruncation {
			log.Printf("[DEBUG] Iter %d: Data size %d exceeds limit %d, attempting truncation.", iterations, currentSize, limit)
		}

		// Find the current longest truncatable string
		// In future, could modify this to exclude paths already tried in this loop pass
		path, longestStr, found := findLongestTruncatableString(*data, nil, minTruncateLength)
		if !found {
			if debugTruncation {
				log.Printf("[DEBUG] Iter %d: Data size %d still exceeds limit %d, but no more truncatable strings found.", iterations, currentSize, limit)
			}

			// If no more strings can be truncated, try to handle nested structures
			nestedTruncated := handleNestedStructures(data, limit, currentSize)
			if nestedTruncated {
				truncated = true

				// Recalculate size after nested structure truncation
				newSize, err := estimateSize(*data)
				if err != nil {
					return truncated, fmt.Errorf("iter %d: failed to estimate size after nested truncation: %w", iterations, err)
				}

				if debugTruncation {
					log.Printf("[DEBUG] Iter %d: After nested truncation. Prev size: %d, New size: %d (Limit: %d)",
						iterations, currentSize, newSize, limit)
				}

				if newSize >= currentSize {
					// Break if size didn't decrease
					if debugTruncation {
						log.Printf("[WARN] Iter %d: Nested truncation didn't reduce size (%d -> %d)",
							iterations, currentSize, newSize)
					}
					break
				}

				currentSize = newSize
				continue // Continue the loop with the new size
			}

			// If no nested structures were truncated, break the loop
			break
		}

		// Simple truncation strategy: truncate to minTruncateLength + ellipsis
		// This is predictable, though potentially aggressive.
		truncatedStr := longestStr[:minTruncateLength] + ellipsis

		if debugTruncation {
			log.Printf("[DEBUG] Iter %d: Attempting to truncate path %v ('%s' len %d) to '%s' (len %d)", iterations, pathToString(path), longestStr, len(longestStr), truncatedStr, len(truncatedStr))
		}

		// Update the map with the truncated string
		if !updateValueByPath(data, path, truncatedStr) {
			// This should ideally not happen if findLongestString returned a valid path
			return truncated, fmt.Errorf("iter %d: failed to update value at path %v", iterations, pathToString(path))
		}
		truncated = true // Mark that at least one truncation happened

		// Recalculate size
		newSize, err := estimateSize(*data)
		if err != nil {
			return truncated, fmt.Errorf("iter %d: failed to estimate size after truncation: %w", iterations, err)
		}

		prevSize := currentSize
		currentSize = newSize

		if debugTruncation {
			log.Printf("[DEBUG] Iter %d: Truncated path %v. Prev size: %d, New size: %d (Limit: %d)", iterations, pathToString(path), prevSize, currentSize, limit)
		}

		// Optional: Check if size actually decreased. If not for several iterations, maybe break?
		// For now, rely on maxIterations and finding no more truncatable strings.
	}

	if iterations == maxTruncateIterations && currentSize > limit {
		if debugTruncation {
			log.Printf("[WARN] Truncation loop reached max iterations (%d) but size %d still exceeds limit %d.", maxTruncateIterations, currentSize, limit)
		}
	}

	if currentSize > limit && !truncated {
		// If we are still over the limit but never truncated anything (e.g., all strings were too short)
		if debugTruncation {
			log.Printf("[WARN] Data size %d still exceeds limit %d, but no truncation was possible.", currentSize, limit)
		}
	} else if currentSize > limit && truncated {
		// If we are still over the limit after truncation attempts
		if debugTruncation {
			log.Printf("[WARN] Data size %d still exceeds limit %d after %d truncation attempts.", currentSize, limit, iterations)
		}
	}

	return truncated, nil
}

// handleNestedStructures handles the truncation of complex nested structures.
// It decides which nested arrays or objects to truncate based on size.
// Returns true if any truncation was done.
func handleNestedStructures(data *map[string]interface{}, limit int64, currentSize int64) bool {
	if debugTruncation {
		log.Printf("[DEBUG] Handling nested structures, currentSize: %d, limit: %d", currentSize, limit)
	}

	// Find the largest nested structure (array or object) to truncate
	path, sizeContribution, found := findLargestNestedStructure(*data, nil, 0)
	if !found || sizeContribution <= 0 {
		if debugTruncation {
			log.Printf("[DEBUG] No suitable nested structures found for truncation")
		}
		return false
	}

	if debugTruncation {
		log.Printf("[DEBUG] Found largest nested structure at path %v with size contribution ~%d bytes",
			pathToString(path), sizeContribution)
	}

	// Get the value at the path
	value, ok := getValueByPath(*data, path)
	if !ok {
		if debugTruncation {
			log.Printf("[ERROR] Failed to get value at path %v", pathToString(path))
		}
		return false
	}

	// Truncate based on the type of the value
	truncated := false

	switch v := value.(type) {
	case []interface{}:
		if len(v) <= 1 {
			// Don't truncate arrays with 0 or 1 elements
			return false
		}

		// Truncate array by removing half of its elements
		newLength := len(v) / 2
		if newLength < 1 {
			newLength = 1
		}

		truncatedArray := v[:newLength]
		if updateValueByPath(data, path, truncatedArray) {
			if debugTruncation {
				log.Printf("[DEBUG] Truncated array at path %v from %d to %d elements",
					pathToString(path), len(v), len(truncatedArray))
			}
			truncated = true
		}

	case map[string]interface{}:
		if len(v) <= 1 {
			// Don't truncate objects with 0 or 1 properties
			return false
		}

		// Find the least important keys to remove (for now, just remove some keys)
		keysToRemove := findKeysToRemove(v)
		if len(keysToRemove) > 0 {
			truncatedMap := make(map[string]interface{})
			for key, val := range v {
				if !containsString(keysToRemove, key) {
					truncatedMap[key] = val
				}
			}

			if updateValueByPath(data, path, truncatedMap) {
				if debugTruncation {
					log.Printf("[DEBUG] Truncated object at path %v by removing %d/%d keys: %v",
						pathToString(path), len(keysToRemove), len(v), keysToRemove)
				}
				truncated = true
			}
		}
	}

	return truncated
}

// findLargestNestedStructure finds the nested structure (array or object) with the largest contribution to size
func findLargestNestedStructure(data interface{}, currentPath []interface{}, depth int) ([]interface{}, int64, bool) {
	if depth > maxNestedDepth {
		return nil, 0, false
	}

	largestPath := make([]interface{}, 0)
	largestSize := int64(0)
	found := false

	switch value := data.(type) {
	case map[string]interface{}:
		// Estimate the size of this map
		mapSize, _ := estimateSize(value)

		// Check if this map is large enough to consider
		if mapSize > largestSize && len(value) > 1 {
			largestPath = currentPath
			largestSize = mapSize
			found = true
		}

		// Recursively check all entries
		for k, v := range value {
			newPath := append(currentPath, k)
			path, size, ok := findLargestNestedStructure(v, newPath, depth+1)
			if ok && size > largestSize {
				largestPath = path
				largestSize = size
				found = true
			}
		}

	case []interface{}:
		// Estimate the size of this array
		arraySize, _ := estimateSize(value)

		// Check if this array is large enough to consider
		if arraySize > largestSize && len(value) > 1 {
			largestPath = currentPath
			largestSize = arraySize
			found = true
		}

		// Recursively check all elements
		for i, v := range value {
			newPath := append(currentPath, i)
			path, size, ok := findLargestNestedStructure(v, newPath, depth+1)
			if ok && size > largestSize {
				largestPath = path
				largestSize = size
				found = true
			}
		}
	}

	return largestPath, largestSize, found
}

// findKeysToRemove determines which keys to remove from an object based on size and importance
func findKeysToRemove(m map[string]interface{}) []string {
	if len(m) <= 1 {
		return nil
	}

	type keySize struct {
		key  string
		size int64
	}

	// Calculate size contribution of each key
	keySizes := make([]keySize, 0, len(m))
	for k, v := range m {
		size, _ := estimateSize(v)
		keySizes = append(keySizes, keySize{key: k, size: size})
	}

	// Sort by size in descending order (for simplicity - could be more sophisticated)
	// For now, just find the half of keys that contribute the most to size
	numToRemove := len(m) / 2
	if numToRemove < 1 {
		numToRemove = 1
	}

	// This is a simple implementation - we could use a better algorithm
	// For now, just return some keys to remove (half of them)
	result := make([]string, 0, numToRemove)
	count := 0
	for _, ks := range keySizes {
		if count >= numToRemove {
			break
		}
		result = append(result, ks.key)
		count++
	}

	return result
}

// containsString checks if a string is in a slice of strings
func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

// estimateSize estimates the size of the data without marshalling to JSON.
// PERFORMANCE: Uses recursive traversal instead of json.Marshal to avoid repeated allocations
func estimateSize(data interface{}) (int64, error) {
	return estimateSizeRecursive(data), nil
}

// estimateSizeRecursive recursively estimates JSON size without marshaling
// This is much faster than json.Marshal for repeated size checks during truncation
func estimateSizeRecursive(data interface{}) int64 {
	switch v := data.(type) {
	case nil:
		return 4 // "null"

	case bool:
		if v {
			return 4 // "true"
		}
		return 5 // "false"

	case int:
		return int64(len(fmt.Sprintf("%d", v)))
	case int8:
		return int64(len(fmt.Sprintf("%d", v)))
	case int16:
		return int64(len(fmt.Sprintf("%d", v)))
	case int32:
		return int64(len(fmt.Sprintf("%d", v)))
	case int64:
		return int64(len(fmt.Sprintf("%d", v)))
	case uint:
		return int64(len(fmt.Sprintf("%d", v)))
	case uint8:
		return int64(len(fmt.Sprintf("%d", v)))
	case uint16:
		return int64(len(fmt.Sprintf("%d", v)))
	case uint32:
		return int64(len(fmt.Sprintf("%d", v)))
	case uint64:
		return int64(len(fmt.Sprintf("%d", v)))

	case float32:
		return int64(len(fmt.Sprintf("%g", v)))
	case float64:
		return int64(len(fmt.Sprintf("%g", v)))

	case string:
		// Account for quotes and escaped characters
		// Approximate: assume each character might need escaping (worst case)
		escaped := 0
		for _, c := range v {
			if c == '"' || c == '\\' || c == '\n' || c == '\r' || c == '\t' {
				escaped++
			}
		}
		return int64(len(v) + escaped + 2) // +2 for quotes

	case map[string]interface{}:
		if len(v) == 0 {
			return 2 // "{}"
		}
		size := int64(2) // "{" and "}"
		first := true
		for key, val := range v {
			if !first {
				size++ // comma
			}
			first = false
			// Key size (quoted + escaped)
			keyEscaped := 0
			for _, c := range key {
				if c == '"' || c == '\\' || c == '\n' || c == '\r' || c == '\t' {
					keyEscaped++
				}
			}
			size += int64(len(key) + keyEscaped + 2) // +2 for quotes
			size++                                   // colon
			size += estimateSizeRecursive(val)
		}
		return size

	case []interface{}:
		if len(v) == 0 {
			return 2 // "[]"
		}
		size := int64(2) // "[" and "]"
		for i, val := range v {
			if i > 0 {
				size++ // comma
			}
			size += estimateSizeRecursive(val)
		}
		return size

	default:
		// Fallback to json.Marshal for unknown types
		bytes, err := json.Marshal(data)
		if err != nil {
			return 0
		}
		return int64(len(bytes))
	}
}

// findLongestTruncatableString recursively searches the data structure for the longest string
// value that is longer than minLen and returns its path and value.
func findLongestTruncatableString(data interface{}, currentPath []interface{}, minLen int) ([]interface{}, string, bool) {
	longestStr := ""
	var longestPath []interface{}
	found := false

	switch value := reflect.ValueOf(data); value.Kind() {
	case reflect.Map:
		if mapData, ok := data.(map[string]interface{}); ok {
			for k, v := range mapData {
				newPath := append(currentPath, k)
				path, str, ok := findLongestTruncatableString(v, newPath, minLen)
				if ok && len(str) > len(longestStr) {
					longestStr = str
					longestPath = path
					found = true
				}
			}
		}
	case reflect.Slice, reflect.Array:
		if sliceData, ok := data.([]interface{}); ok {
			for i, v := range sliceData {
				newPath := append(currentPath, i)
				path, str, ok := findLongestTruncatableString(v, newPath, minLen)
				if ok && len(str) > len(longestStr) {
					longestStr = str
					longestPath = path
					found = true
				}
			}
		}
	case reflect.String:
		str := value.String()
		if len(str) > minLen && len(str) > len(longestStr) { // Check against minLen here
			longestStr = str
			longestPath = currentPath
			found = true
		}
	}

	return longestPath, longestStr, found
}

// updateValueByPath updates the value within a nested structure (maps/slices)
// using the provided path. Returns true if update was successful.
func updateValueByPath(data *map[string]interface{}, path []interface{}, newValue interface{}) bool {
	if data == nil || *data == nil || len(path) == 0 {
		return false
	}

	var current interface{} = *data // Start with the map itself (which is map[string]interface{})

	for i, keyOrIndex := range path {
		isLast := i == len(path)-1

		switch currentVal := current.(type) {
		case map[string]interface{}:
			key, ok := keyOrIndex.(string)
			if !ok {
				if debugTruncation {
					log.Printf("[ERROR] Path element %v (type %T) is not a string key for map at path index %d", keyOrIndex, keyOrIndex, i)
				}
				return false
			}
			if isLast {
				currentVal[key] = newValue
				return true
			}
			nextLevel, exists := currentVal[key]
			if !exists {
				if debugTruncation {
					log.Printf("[ERROR] Path key '%s' does not exist in map at path index %d", key, i)
				}
				return false
			}
			// Ensure next level is navigable
			if _, okMap := nextLevel.(map[string]interface{}); !okMap {
				if _, okSlice := nextLevel.([]interface{}); !okSlice {
					if debugTruncation {
						log.Printf("[ERROR] Cannot navigate path further at map key '%s' (index %d): unsupported type %T", key, i, nextLevel)
					}
					return false // Cannot navigate into this type
				}
			}
			current = nextLevel // Move deeper

		case []interface{}:
			index, ok := keyOrIndex.(int)
			if !ok {
				if debugTruncation {
					log.Printf("[ERROR] Path element %v (type %T) is not an int index for slice at path index %d", keyOrIndex, keyOrIndex, i)
				}
				return false
			}
			if index < 0 || index >= len(currentVal) {
				if debugTruncation {
					log.Printf("[ERROR] Path index %d out of bounds for slice at path index %d (len %d)", index, i, len(currentVal))
				}
				return false
			}
			if isLast {
				currentVal[index] = newValue
				return true
			}
			nextLevel := currentVal[index]
			// Ensure next level is navigable
			if _, okMap := nextLevel.(map[string]interface{}); !okMap {
				if _, okSlice := nextLevel.([]interface{}); !okSlice {
					if debugTruncation {
						log.Printf("[ERROR] Cannot navigate path further at slice index %d (index %d): unsupported type %T", index, i, nextLevel)
					}
					return false // Cannot navigate into this type
				}
			}
			current = nextLevel // Move deeper

		default:
			// This should not happen if findLongestString returns valid paths within maps/slices
			if debugTruncation {
				log.Printf("[ERROR] Cannot navigate path further at index %d: unsupported type %T for element %v", i, currentVal, keyOrIndex)
			}
			return false
		}
	}

	// Should not be reached if path leads to a value successfully
	if debugTruncation {
		log.Printf("[ERROR] Path update failed unexpectedly after loop completion.")
	}
	return false
}

// Utility function to convert path to string for logging
func pathToString(path []interface{}) string {
	var parts []string
	for _, p := range path {
		parts = append(parts, fmt.Sprintf("%v", p))
	}
	return strings.Join(parts, " ")
}

// Helper to get value by path - useful for debugging or verification
func getValueByPath(data interface{}, path []interface{}) (interface{}, bool) {
	current := data
	for i, keyOrIndex := range path {
		switch currentVal := current.(type) {
		case map[string]interface{}:
			key, ok := keyOrIndex.(string)
			if !ok {
				return nil, false
			} // Invalid path
			if nextVal, exists := currentVal[key]; exists {
				current = nextVal
			} else {
				return nil, false // Path doesn't exist
			}
		case []interface{}:
			index, ok := keyOrIndex.(int)
			if !ok || index < 0 || index >= len(currentVal) {
				return nil, false
			} // Invalid path
			current = currentVal[index]
		default:
			// If not the last element, this is an error in the path or structure
			if i != len(path)-1 {
				return nil, false
			}
			// If it's the last element, we've found the value
		}
	}
	return current, true
}

// --- Removed previous version of truncateValue ---
