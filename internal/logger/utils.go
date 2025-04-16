package logger

// truncateString truncates a string to a maximum length, adding an ellipsis if truncated.
// Used by both file and gelf loggers.
func truncateString(s string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	const ellipsis = "...truncated"
	if len(s) <= maxLength {
		return s
	}
	if maxLength <= len(ellipsis) {
		return s[:maxLength] // Not enough space for ellipsis, just cut
	}
	return s[:maxLength-len(ellipsis)] + ellipsis
}
