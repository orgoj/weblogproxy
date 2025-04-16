package version

// Version information
var (
	// Version is the current version of weblogproxy
	Version = "0.9.4-dev"
	// BuildDate is the date when the binary was built
	BuildDate = "undefined"
	// CommitHash is the git commit hash when the binary was built
	CommitHash = "undefined"
)

// VersionInfo returns formatted version information
func VersionInfo() string {
	return "WebLogProxy version " + Version + " (build: " + BuildDate + ", commit: " + CommitHash + ")"
}
