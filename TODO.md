# TODO List
---
- **IMPORTANT**: ALWAYS READ RULES @todo_rules.mdc BEFORE STARTING WORK
---

## HIGH PRIORITY

## LOW PRIORITY

- [ ] Improve validation of config values (e.g., regex patterns in rules?) on load

## Tests

- [ ] Implement systematic validation of log file content (JSON/Text) in E2E tests (using jq/grep/awk in run.sh or helper scripts).
- [-] Test output file formats (JSON/Text) by reading log files after E2E test.

- [ ] Add E2E test for GELF destination, including verification of sent data (requires mock GELF server) and compression options.
- [ ] Test output for GELF destination when compression is implemented

- [-] Extend E2E tests for security aspects: test rate limit boundaries and max body size limits precisely.
- [-] Extend E2E tests for more complex `log_config` scenarios (multiple rules, `add_log_data`, combinations of conditions).

## Unit / Integration Tests

- [ ] Add more unit tests for rule matching logic and enricher, focusing on edge cases (empty values, special chars, large volumes).
- [ ] Consider adding Go-based integration tests for RuleProcessor -> Enricher -> LoggerManager flow (with mocked loggers).

## Maybe we'll do

- [-] Optimize system resource consumption (not started).
- [?] Add basic load tests.
- [?] Optional javascript template in condition
- [?] Add basic load tests.
- [?] Error log to file instead of stderr, not a problem for docker now, could be log_destination.name=internal later, which would support rotation
- [?] Setup https://www.coderabbit.ai/
