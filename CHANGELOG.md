# Changelog

## [Unreleased]

### Added
- 

### Changed
- 

### Fixed
- Gin framework nyní vždy běží v release (production) módu, odstraněny debug hlášky v konzoli
- Logování HTTP požadavků nyní respektuje nastavenou úroveň v app_log.level díky slog-gin middleware


## [0.11.0] - 2025-04-17

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
- Resolved various test failures caused by missing `AppLogger` initialization.
- Corrected `truncateString` suffix to `...truncated` for compatibility with tests.


## [0.10.2] - 2025-04-17

### Fixed
- Github CI


## [0.10.0] - 2025-04-16

### Added
- Implemented GELF Logger for sending logs to Graylog servers with support for both UDP and TCP protocols
- Added server configuration for rate limits, CORS, additional HTTP headers
- Implemented compression for GELF (gzip, zlib, none) with unit tests
- Implemented GELF message truncation based on `max_message_size` configuration.
- Implemented message truncation for File logger (text and JSON formats) based on `max_message_size` configuration.


## [0.9.4] - 2025-04-16

### Added
- Linting setup: project now uses golangci-lint for static code analysis. All lint errors fixed.
- Automatic Go code formatting: added mise task 'format' using gofmt and VSCode integration for format on save.
- Added automated security checks (`gosec`, `govulncheck`) integrated into the build process via `mise` tasks.


## [0.9.3] - 2025-04-16

- Test bump


## [0.9.2] - 2025-04-16

- Fix blank logger.js template
- Fix version in docker log
- Updated docker-build.sh and docker-test.sh scripts to accept full image name including tag
- Modified logger.js template to avoid generating scriptsToInject when none are defined
- Added server.protocol configuration option to control URL schema (http/https)
- Changed JavaScript global object name from window.weblogproxy to window.wlp and made it configurable
- Improved entrypoint.sh to correctly detect binary paths when running in Docker


## [0.9.1] - 2025-04-10

- Docker file use last version of images


## [0.9.0] - 2025-04-04

First public pre-release version.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

- Improved documentation for rate limiting: added YAML example and detailed explanation in README and example config.
