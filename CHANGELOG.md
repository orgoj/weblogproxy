# Changelog

## [0.9.3] - 2025-04-16
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
