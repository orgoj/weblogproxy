package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	DefaultMaxInputLength = 256
	DefaultMaxDepth       = 10
	DefaultMaxKeyLength   = 64
)

// Regex for basic validation of IDs (alphanumeric, underscore, hyphen)
var idRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ErrInputTooLong indicates the input string exceeds the maximum allowed length.
var ErrInputTooLong = errors.New("input exceeds maximum length")

// ErrInvalidChars indicates the input string contains disallowed characters.
var ErrInvalidChars = errors.New("input contains invalid characters")

// ErrMaxDepthExceeded indicates the nested structure exceeds the maximum allowed depth.
var ErrMaxDepthExceeded = errors.New("maximum nesting depth exceeded")

// IsValidID checks if the input string matches the allowed format for site_id/gtm_id.
func IsValidID(id string, maxLength int) error {
	if len(id) > maxLength {
		return fmt.Errorf("%w: got %d, max %d", ErrInputTooLong, len(id), maxLength)
	}
	if !idRegex.MatchString(id) {
		return fmt.Errorf("%w: allowed alphanumeric, underscore, hyphen", ErrInvalidChars)
	}
	return nil
}

// SanitizeString removes non-printable characters (excluding space) and trims whitespace.
// It also truncates the string to maxLength.
func SanitizeString(s string, maxLength int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLength {
		s = s[:maxLength]
	}
	// Remove non-printable characters except space
	s = strings.Map(func(r rune) rune {
		if r == ' ' || (unicode.IsPrint(r) && r != '\uFFFD') { // Keep space and printable runes (avoid replacement char)
			return r
		}
		return -1 // Remove other runes
	}, s)
	return s
}

// SanitizeMapRecursively sanitizes keys and string values within a nested map.
// It limits nesting depth and key/string lengths.
func SanitizeMapRecursively(data map[string]interface{}, maxDepth, currentDepth, maxKeyLength, maxStringLength int) (map[string]interface{}, error) {
	if currentDepth > maxDepth {
		return nil, ErrMaxDepthExceeded
	}
	if data == nil {
		return nil, nil
	}

	sanitizedMap := make(map[string]interface{}, len(data))

	for key, value := range data {
		// Sanitize key (length, printable)
		sanitizedKey := SanitizeString(key, maxKeyLength)
		// Optional: Further validation on key characters if needed
		if sanitizedKey == "" {
			continue
		} // Skip empty keys after sanitization

		switch v := value.(type) {
		case string:
			sanitizedMap[sanitizedKey] = SanitizeString(v, maxStringLength)
		case map[string]interface{}:
			nestedMap, err := SanitizeMapRecursively(v, maxDepth, currentDepth+1, maxKeyLength, maxStringLength)
			if err != nil {
				return nil, fmt.Errorf("error sanitizing nested map under key '%s': %w", sanitizedKey, err)
			}
			sanitizedMap[sanitizedKey] = nestedMap
		case []interface{}:
			// Handle slices/arrays if necessary (e.g., sanitize strings within them)
			// For now, pass them through or sanitize recursively if elements can be maps/strings
			sanitizedSlice, err := SanitizeSliceRecursively(v, maxDepth, currentDepth+1, maxKeyLength, maxStringLength)
			if err != nil {
				return nil, fmt.Errorf("error sanitizing slice under key '%s': %w", sanitizedKey, err)
			}
			sanitizedMap[sanitizedKey] = sanitizedSlice
		default:
			// Keep numbers, booleans, nulls as they are
			sanitizedMap[sanitizedKey] = v
		}
	}
	return sanitizedMap, nil
}

// SanitizeSliceRecursively sanitizes elements within a slice, similar to map sanitization.
func SanitizeSliceRecursively(data []interface{}, maxDepth, currentDepth, maxKeyLength, maxStringLength int) ([]interface{}, error) {
	if currentDepth > maxDepth {
		return nil, ErrMaxDepthExceeded
	}
	if data == nil {
		return nil, nil
	}

	sanitizedSlice := make([]interface{}, len(data))
	for i, item := range data {
		switch v := item.(type) {
		case string:
			sanitizedSlice[i] = SanitizeString(v, maxStringLength)
		case map[string]interface{}:
			nestedMap, err := SanitizeMapRecursively(v, maxDepth, currentDepth+1, maxKeyLength, maxStringLength)
			if err != nil {
				return nil, fmt.Errorf("error sanitizing map in slice index %d: %w", i, err)
			}
			sanitizedSlice[i] = nestedMap
		case []interface{}:
			nestedSlice, err := SanitizeSliceRecursively(v, maxDepth, currentDepth+1, maxKeyLength, maxStringLength)
			if err != nil {
				return nil, fmt.Errorf("error sanitizing nested slice in slice index %d: %w", i, err)
			}
			sanitizedSlice[i] = nestedSlice
		default:
			sanitizedSlice[i] = v // Keep other types
		}
	}
	return sanitizedSlice, nil
}
