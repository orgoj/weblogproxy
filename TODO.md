# TODO List
---
- **IMPORTANT**: ALWAYS READ RULES @todo_rules.mdc BEFORE STARTING WORK
---

## HIGH PRIORITY

## E2E Tests

- [-] Test output file formats (JSON/Text) by reading log files after E2E test.
- [-] Extend E2E tests for more complex `log_config` scenarios (multiple rules, `add_log_data`, combinations of conditions).
- [-] Extend E2E tests for security aspects (rate limit boundaries, max body size, token edge cases). 
- [ ] Improve validation of config values (e.g., regex patterns in rules?)
- [ ] Add more unit tests, especially for rule matching logic and enricher
- [ ] Test output for GELF destination when compression is implemented

## Maybe we'll do

- [-] Optimize system resource consumption (not started). 
- [?] Optional javascript template in condition
- [?] Add basic load tests.
- [?] Error log to file instead of stderr, not a problem for docker now, could be log_destination.name=internal later, which would support rotation
- [?] Setup https://www.coderabbit.ai/