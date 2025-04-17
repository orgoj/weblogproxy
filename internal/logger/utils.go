package logger

// truncateString truncates a string to the specified maximum length.
// If the string is longer than maxLength, it will be truncated and "...truncated" will be appended.
func truncateString(s string, maxLength int) string {
	if len(s) <= maxLength {
		return s
	}

	// Define ellipsis
	const ellipsis = "...truncated"

	// Leave space for the ellipsis
	if maxLength <= len(ellipsis) {
		// Not enough space for ellipsis, just cut
		return s[:maxLength]
	}

	return s[:maxLength-len(ellipsis)] + ellipsis
}
