# TODO List for weblogproxy (RULE: DONE task write to @CHANGELOG.md and delete from here)

## GELF (LATER after everything else works and all tests)

- [ ] Implement log truncation based on MaxBodySize for GELF and other formats

## E2E Tests

- [-] Test output file formats (JSON/Text) by reading log files after E2E test.
- [-] Extend E2E tests for more complex `log_config` scenarios (multiple rules, `add_log_data`, combinations of conditions).
- [-] Extend E2E tests for security aspects (rate limit boundaries, max body size, token edge cases). 
- [ ] Improve validation of config values (e.g., regex patterns in rules?)
- [ ] Add more unit tests, especially for rule matching logic and enricher

## Maybe we'll do

- [~] Extend documentation in `README.md` (ONLY AT THE END WHEN EVERYTHING IS READY)
- [~] Upload to Github
- [~] Upload to docker hub, or maybe github to your docker hub
- [-] Optimize system resource consumption (not started). 
- [ ] Implement rate limiting (e.g., using `golang.org/x/time/rate`) (LATER handled by reverse proxy)
- [?] Optional javascript template in condition
- [?] Test output for GELF destination (once implemented).
- [?] Add basic load tests.
- [?] Error log to file instead of stderr, not a problem for docker now, could be log_destination.name=internal later, which would support rotation
