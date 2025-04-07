package version

import (
	"strings"
	"testing"
)

func TestVersionInfo(t *testing.T) {
	info := VersionInfo()

	// Verify format of version info
	if !strings.Contains(info, "WebLogProxy version") {
		t.Errorf("Expected version info to contain 'WebLogProxy version', got: %s", info)
	}

	if !strings.Contains(info, Version) {
		t.Errorf("Expected version info to contain version number '%s', got: %s", Version, info)
	}

	if !strings.Contains(info, "build:") {
		t.Errorf("Expected version info to contain build date information, got: %s", info)
	}

	if !strings.Contains(info, "commit:") {
		t.Errorf("Expected version info to contain commit hash information, got: %s", info)
	}
}
