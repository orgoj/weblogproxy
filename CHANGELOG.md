# Changelog

## [Unreleased]

### Added
- Added configuration option `server.health_allowed_ips` to restrict health endpoints based on CIDR ranges with middleware and tests
- Introduced `AppLogger` for standardized application-level logging to stdout.
- Added configuration options `app_log.level` and `app_log.show_health_logs`.
- Added configurable `server.unknown_route.code` and `server.unknown_route.cache_control` for unknown routes (default: 200, 1 hour cache, blank body)
- Added Google Tag Manager integration example and template (`gtm_tag_template.html`)

### Changed
- Replaced internal `fmt.Printf` and direct `os.Stderr` writes with `AppLogger` across the codebase.
- Updated documentation in README.md to include `server.health_allowed_ips` option under server configuration

### Fixed
- Resolved various test failures caused by missing `