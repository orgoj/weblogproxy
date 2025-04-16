# Changelog

## [Unreleased]

### Added
- Implemented GELF Logger for sending logs to Graylog servers with support for both UDP and TCP protocols
- Added server configuration for rate limits, CORS, additional HTTP headers
- Implemented compression for GELF (gzip, zlib, none) with unit tests
- Implemented GELF message truncation based on `max_message_size` configuration.
- Implemented message truncation for File logger (text and JSON formats) based on `max_message_size` configuration.

### Changed
- 

### Fixed
- 


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
