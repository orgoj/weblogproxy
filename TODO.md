# TODO List for weblogproxy

## URGENT

- [ ] mise run lint
- [ ] mise run docs - je to k necemu, bude to neco generovat, chybi swag
- [ ] na co je config-validator, config si ma testovat sama aplikace na option, tuto funkci musi mit aby se dal udelat reload configu

- [x] Headers should be in condition and behave same as add_log_data
- [x] How to remove a condition value for header or add_log_data, it should be possible to completely remove it with a false value

- [x] Extend tests for CORS functionality verification, especially preflight requests
- [x] Implement validation for CORS settings when loading configuration
- [x] Add integration tests for CORS configuration

## Project Structure

- [x] Dockerfile
- [x] Github CI
- [~] Extend documentation in `README.md` (ONLY AT THE END WHEN EVERYTHING IS READY)
- [~] Upload to Github
- [~] Upload to docker hub, or maybe github to your docker hub
- [x] Add versioning and CHANGELOG.md with proper maintenance procedures

## Core Application

- [x] Data truncation (`truncateLogDataIfNeeded`) should support nested structures.

## Testing

### E2E Tests

- [-] Test output file formats (JSON/Text) by reading log files after E2E test.
- [-] Extend E2E tests for more complex `log_config` scenarios (multiple rules, `add_log_data`, combinations of conditions).
- [-] Extend E2E tests for security aspects (rate limit boundaries, max body size, token edge cases). 
- [ ] Improve validation of config values (e.g., regex patterns in rules?)
- [ ] Add more unit tests, especially for rule matching logic and enricher
- [x] Add integration tests for different header configurations
- [x] Add integration tests for trusted proxies and IP detection

## GELF (LATER after everything else works and all tests)

- [-] Implement GELF Logger
- [ ] Add support for TCP GELF
- [ ] Add support for ZLIB GELF compression
- [ ] Implement log truncation based on MaxBodySize for GELF and other formats

## Maybe we'll do

- [?] Optional javascript template in condition
- [-] Optimize system resource consumption (not started). 
- [?] Test output for GELF destination (once implemented).
- [?] Add basic load tests.
- [?] Error log to file instead of stderr, not a problem for docker now, could be log_destination.name=internal later, which would support rotation
- [ ] Implement rate limiting (e.g., using `golang.org/x/time/rate`) (LATER handled by reverse proxy)
