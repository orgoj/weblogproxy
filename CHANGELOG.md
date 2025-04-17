# Changelog

## [Unreleased]

### Added
- Added configuration option `server.health_allowed_ips` to restrict health endpoints based on CIDR ranges with middleware and tests
- Introduced `AppLogger` for standardized application-level logging to stdout.
- Added configuration options `app_log.level` and `app_log.show_health_logs`.

### Changed
- Replaced internal `fmt.Printf` and direct `os.Stderr` writes with `AppLogger` across the codebase.
- Updated documentation in README.md to include `server.health_allowed_ips` option under server configuration

### Fixed
- Resolved various test failures caused by missing `