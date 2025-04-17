# Testing Strategy in WebLogProxy

This document outlines the testing strategy and implementation details for the WebLogProxy project.

## Overview

The project employs a multi-layered testing approach to ensure code quality, functionality, and robustness:

1.  **Unit Tests (Go):** Focus on testing individual functions, methods, and smaller components in isolation. Located in `*_test.go` files within the corresponding `internal/` subpackages.
2.  **Integration Tests (Go):** Some unit tests have an integration character, testing the interaction between a few closely related components (e.g., middleware tests).
3.  **End-to-End (E2E) Tests:** Verify the complete application flow by interacting with the running application (usually via Docker) through its HTTP API. Located primarily in the `test/` directory.

## Unit & Integration Tests (Go)

-   **Location:** `internal/**/*.go` (`*_test.go` files)
-   **Framework:** Standard Go `testing` package, `stretchr/testify` (for assertions and requirements).
-   **Coverage:**
    -   **Configuration (`internal/config`):** Extensive testing of loading valid and invalid YAML configurations, parsing custom types (duration, size), and validation logic (including CORS, unknown route handling).
    -   **Security (`internal/security`):** Thorough testing of JWT token generation and validation, including expiration, different claims, and error handling.
    -   **Middleware (`internal/server`):** Tests cover rate limiting logic (using `golang.org/x/time/rate`, testing different scenarios with/without proxies, limit exceeding) and IP filtering for the health endpoint.
    -   **Rule Processor (`internal/rules`):** Comprehensive tests covering various rule matching scenarios (SiteID, IP, UserAgent, CIDR, XFF header), `continue` flag behavior, data/script accumulation and overwriting, destination handling, header conditions, and disabled rules.
    -   **Enricher (`internal/enricher`):** Tests basic data merging, enrichment logic, and field removal.
    -   **Loggers (`internal/logger`):**
        -   *File Logger:* Tests initialization, basic JSON/Text logging, and detailed message truncation logic when size limits are exceeded. Rotation tests are currently skipped due to library limitations in tests.
        -   *GELF Logger:* Tests configuration validation, log level transformation, message truncation logic, and compression. **Does not test actual network sending.**
        -   *Manager:* Tests dynamic initialization of loggers based on configuration.
    -   **Truncate (`internal/truncate`):** Very detailed tests covering various scenarios of message truncation (strings, arrays, objects, nested structures) based on byte size limits.
    -   **Handlers (`internal/handler/test`):** Basic validation tests for `/logger.js` request parameters.
    -   **Utilities:** Tests for `version` and `iputil` packages.
-   **Execution:** Run via `go test ./...` or `mise run test-unit`.

## End-to-End (E2E) Tests

-   **Location:** `test/` directory, primarily `test/api/`.
-   **Frameworks/Tools:**
    -   `hurl`: For defining and executing HTTP requests and asserting responses (status, headers, body).
    -   Shell Scripts (`bash`): Used for more complex test orchestration, setup, teardown, generating tokens, and validating side effects (like log file content).
    -   `jq`: Used within shell scripts for parsing and asserting JSON content in responses or log files.
    -   `docker` / `docker-compose`: Used to build and run the application container for testing.
    -   `mise`: Used to manage tasks, including running E2E tests (`mise run test-e2e`, `mise run docker-test`).
-   **Structure:**
    -   `test/run.sh`: Main script orchestrating the E2E tests. Sets up environment, runs `hurl` tests, executes `.sh` test scripts, performs cleanup.
    -   `test/api/*.hurl`: Define individual HTTP requests and basic assertions.
    -   `test/api/*.sh`: Implement more complex test logic, often invoking `hurl` for specific requests and then performing additional checks.
    -   `test/api/e2e_env.vars`: Variables used by `hurl` tests (e.g., base URL).
-   **Coverage:**
    -   **API Endpoints:** `/logger.js` (valid/invalid), `/log` (valid/invalid token, data), `/health`, `/version`. Unknown routes (checking configured status code and cache header).
    -   **Configuration Scenarios:** Standalone vs. Embedded mode, CORS, `server.headers`, IP detection (`X-Forwarded-For` with `trusted_proxies`).
    -   **Logging Scenarios:** Tests using `config/test.yaml` covering multiple rules, `add_log_data`, header conditions (`05_log_scenarios.hurl`, `09_header_config.hurl`).
    -   **Security:** Invalid tokens, CORS requests, rate limit boundaries, max body size.
    -   **Side Effects:**
        -   *Log Rotation:* Specific test (`06_log_rotation.sh`, `*.hurl`).
        -   *Log File Content:* Some `.sh` scripts (`09_header_config.sh`) use `grep` to validate basic content in generated log files. This validation needs to be made more systematic.
-   **Execution:** Run via `mise run test-e2e` (runs `test/run.sh` directly) or `mise run docker-test` (builds image, runs container, then executes tests against the container).

## Current Status & Areas for Improvement

(Refer to `TODO.md` for actionable items)

-   The testing suite provides good coverage for core logic, configuration parsing, security aspects (tokens, limits), and basic API functionality.
-   Unit tests for `truncate`, `rules`, and `config` are particularly comprehensive.
-   E2E tests validate the main user flows and several configuration aspects.

**Identified Gaps / Potential Improvements (See TODO.md):**

1.  **Systematic E2E Log Validation:** Implement consistent checks of log file content.
2.  **E2E Test for GELF:** Add an E2E test scenario with a mock GELF server.
3.  **Unit Test Expansion:** Add more edge case tests (Rule Processor, Enricher).
4.  **Integration Tests:** Consider adding Go-based integration tests (RuleProcessor -> Enricher -> LoggerManager).
5.  **Load Tests:** Implement basic load tests.
