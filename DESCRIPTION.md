# weblogproxy - HTTP Logging Proxy Service Specification
    - ## Basic Goal
		- Easy logging from Google tag manager (GTM) for any web frontendapplication
		- Configurable loggin activation for some or all web users
		- Selectable log destinations for each web user individually
		- Log to file eady readeble with Lnav
		- Log to Elastic via GELF
		- Single app in Docker container with no depedencies like log rotation
		- Secure, prevent logging everyvone
		- Silent for web user, no log or error in browser
		- Easy frontend implementation
			- include single script
			- call log function with any data without if
		- Usable as central frontend log for multiple websites
	- ## Project Overview
		- Centralized, configurable logging service for web applications
		- Provide secure, flexible logging mechanism via JavaScript script
		- Designed for **extensibility** to support new logging destinations easily.
	- ## Technical Specification
		- ### Technology Stack
			- Language: Go (Golang) version 1.21 or later recommended
			- Web Framework: Gin
			- Containerization: Docker
			- Configuration: YAML
			- Testing:
				- Go testing framework for unit, integration tests
				- Shell script and Hurl for E2E tests
				- jq for JSON processing
			- Development Language: English for all code, comments, and documentation
		- ### System Components
			- #### Configuration File YAML
				- Defines logging rules, permissions, and data enrichment.
				- Contains sections for:
					- `config_reload`: Optional dynamic config reload settings (enabled, interval).
					- `server`: Server settings including:
						- `host`: hostname or IP address to bind to. 
						- `port`: port to listen on.
						- `mode`: 'standalone' or 'embedded'.
						- `domain`: Full domain name for standalone mode (only used in 'standalone' mode).
						- `path_prefix`: URL path prefix for embedded mode (only used in 'embedded' mode).
						- `cors` settings (enabled, allowed_origins, max_age).
						- `headers` (for caching `/logger.js`).
						- `request_limits` (max_body_size, rate_limit).
						- `trusted_proxies` (list of trusted reverse proxy IPs/CIDRs for evaluating `X-Forwarded-For` header).
					- `security`: Security settings, primarily `token` configuration (secret, expiration). Token expiration is specified in a duration format (e.g., "30m", "1h") rather than having the unit in the key name.
					- `log_config`: A list of logging rules processed sequentially. Each rule contains:
						- `condition`: Matching criteria for the rule, including:
							- `site_id`: Match by site identifier.
							- `gtm_ids`: List of GTM IDs to match.
							- `user_agents`: List of user agent patterns (using glob matching).
							- `ips`: List of IP addresses or CIDR notation IP ranges.
							- `headers`: Map of HTTP headers to match, where:
							    - String value: Header must exist with exact value match
							    - Boolean `true`: Header must exist (any value)
							    - Boolean `false`: Header must NOT exist
						- `enabled`: Boolean to enable/disable this rule.
						- `continue`: Boolean indicating whether processing should continue to the next rule if this one matches. If `false` or not set, processing stops at this rule if it matches.
						- `script_injection`: List of script objects to inject, with fields:
							- `url`: Script URL.
							- `async`: Boolean to set async attribute.
							- `defer`: Boolean to set defer attribute.
						- `add_log_data`: List of data enrichment actions, each containing:
							- `name`: Field name to add/modify in the log.
							- `source`: Source of the data ('static', 'header', 'query', 'post').
							- `value`: Source-specific value:
							    - For 'static', this is the literal value to set.
							    - For 'header', 'query', 'post', this is the key name to extract from the respective source.
							    - Special value "false" with source "static" removes the field specified by "name" from the log record.
						- `log_destinations`: Optional list of destination names (must match entries from `log_destinations`) to send logs to if this rule matches. If absent or empty, logs are sent to all enabled destinations.
					- `log_destinations`: A list of destinations where logs should be sent. Each destination defines:
						- `name`: A unique identifier/name for this destination.
						- `type`: Destination type ('file', 'syslog', or 'gelf'). New types can be added by implementing the Logger interface.
						- `enabled`: Boolean flag to enable/disable the destination.
						- Type-specific settings:
						    - `file`: `path`, `format` ('json' or 'text'), `rotation` (optional: max_size in MB as a string value with minimum 1MB due to Lumberjack library limitations, `max_age` in duration format (e.g., "1d" for 1 day), `max_backups`, `compress`). The 'json' format outputs Bunyan-compatible JSON Lines.
						    - `syslog`: Forwards logs to a syslog server for centralized logging.
						    - `gelf`: `host`, `port`, `protocol` ('udp' or 'tcp'), `compression` ('gzip', 'zlib', or 'none'), `max_message_size` (optional, integer, max bytes per message, default depends on protocol). GELF output adheres to GELF v1.1 specification (e.g., custom fields prefixed with `_`). Also supports optional `additional_fields` to add custom fields to all GELF messages.
						- `add_log_data`: Optional list of actions to enrich log data specific to this destination, applied *after* rule-based enrichment but *before* merging client data. Uses the same structure as `add_log_data` in `log_config`. Can overwrite fields set by rules or add new ones.
			- #### Core Components
				- Config Parser: Loads and validates YAML configuration.
				- Rule Processor: Implements the logic for matching rules, handling `continue`, accumulating `add_log_data` from rules, collecting `script_injection`, and determining target `log_destinations`.
				- Token Manager: Generates and validates security tokens with configurable expiration times.
				- Logger Interface: Abstraction for different logging backends (File, GELF). Designed to allow easy registration and addition of new logger types. Each implementation handles destination-specific formatting (e.g., Bunyan JSON for file, GELF transformation for GELF).
				- Data Enricher/Merger: Constructs the final **internal representation** of the log record by:
				    1. Creating a base record with common fields (timestamp, hostname, pid, level, etc.).
				    2. Applying `add_log_data` from matching rules and the target destination.
				    3. Merging the client-provided JSON data (`data` field from the `/log` request), with client fields overwriting any existing fields from steps 1 & 2.
				- Request Validator: Validates incoming requests and ensures proper sanitization of inputs.
				- IP Utilities: Provides functions for parsing CIDR notations and determining the real client IP based on trusted proxies configuration.
			- #### CORS Implementation
				- ##### Overview
					- Proper CORS support is critical for standalone mode when the client-side JavaScript is hosted on a different domain than the weblogproxy server.
					- CORS is implemented to handle both preflight OPTIONS requests and actual POST requests for logging.
				- ##### CORS Configuration
					- Enabled via `server.cors.enabled` setting in the YAML configuration.
					- Allowed origins are specified in `server.cors.allowed_origins` as an array of domain strings.
					- A wildcard `*` can be used to allow all origins (not recommended for production).
					- Configuration validation ensures:
						- When CORS is enabled, the `allowed_origins` array cannot be empty.
						- Each origin must be properly formatted (starts with "http://" or "https://") unless it's a wildcard "*".
						- The `max_age` value cannot be negative.
				- ##### Preflight Handling
					- OPTIONS requests are automatically handled with appropriate CORS headers:
						- `Access-Control-Allow-Origin`: Set to the matching origin from the allowed list, or not set if origin is not allowed.
						- `Access-Control-Allow-Methods`: Set to "GET, POST, OPTIONS".
						- `Access-Control-Allow-Headers`: Set to "Content-Type, Content-Length, Accept-Encoding, Authorization, X-Requested-With".
						- `Access-Control-Allow-Credentials`: Set to "true".
						- Preflight requests receive a 204 No Content response when successful.
					- The middleware automatically aborts OPTIONS requests from disallowed origins with a 403 Forbidden status.
				- ##### POST Request Handling
					- The same CORS headers are included in POST responses to allow browsers to access the response data.
					- The middleware design allows normal requests to pass through without CORS headers if the origin is not allowed.
				- ##### Security Considerations
					- Origin matching is strict and case-sensitive.
					- Only the specifically listed origins in the configuration are allowed.
					- Wildcard (*) origins should be used carefully as they open up access from any domain.
				- ##### Testing
					- Comprehensive integration tests verify CORS functionality:
						- Preflight OPTIONS requests with various origins.
						- Regular POST requests with CORS headers.
						- Proper header handling for both allowed and disallowed origins.
						- Testing wildcard origin configuration behavior.
			- #### Endpoints
				- ##### `/logger.js`
					- Input Parameters: `site_id`, `gtm_id` (optional).
					- Functionality:
						- Returns HTTP 200 with `Content-Type: application/javascript` and configured cache headers.
						- Evaluates `log_config` rules based on input parameters, User-Agent, and Client IP.
						- Uses a Go template system to generate the JavaScript code, with the template being cached for performance.
						- The script operates silently without console debug logs to ensure quiet operation in production environments.
						- URL generation varies based on server mode:
							- In `embedded` mode: Generates relative URLs for internal endpoints using the configured `path_prefix`.
							- In `standalone` mode: Generates absolute URLs including protocol and the configured `domain`.
						- **If logging is disabled:**
							- Returns a JavaScript with a no-op function that silently ignores all calls, without any configuration object.
							- The log function becomes a no-op function that silently ignores all calls.
							- Still includes scripts configured in matching rules, allowing for script injection independent of logging functionality.
						- **If logging is enabled:**
							- Generates a security token with the configured expiration time.
							- Returns a JavaScript defining `window.wlp` object with:
								- A `log(data)` function: Accepts any data type. If `data` is not an object, it's wrapped into `{ "msg": String(data) }`. The resulting object is sent along with `site_id`, `gtm_id`, and the generated `token` asynchronously via POST request to the `/log` endpoint (under the `data` key in the request body).
						- Generates code for script injection *only if* the accumulated and deduplicated list of scripts from matching rules is not empty. This code dynamically appends the configured script tags to the document.
				- ##### `/log`
					- Functionality:
						- Receives logging data (JSON POST body with `token`, `site_id`, `gtm_id`, and `data` object).
						- Validates security token.
						- Enforces request size limits with proper body size control.
						- Implements rate limiting based on client IP.
						- Evaluates `log_config` rules based on `site_id`, `gtm_id`, User-Agent, and Client IP from the request context.
						- Logs unauthorized access attempts for security monitoring.
						- If logging is enabled by a final matching rule:
							- Determines the target log destinations based on the rule's `log_destinations` list (or all enabled destinations if the list is absent).
							- For each target destination:
							    1. **Internal Record Creation:** A canonical internal log record is created by the Data Enricher/Merger (Base fields + `add_log_data` enrichment + Client data merge with overwrite).
							    2. **Destination Formatting & Forwarding:** The internal record is passed to the Logger Interface implementation for the target destination. This implementation transforms the internal record into the destination-specific format (e.g., Bunyan JSON for file, GELF JSON with field transformations for GELF) and sends it.
				- ##### `/health` 
					- Simple health check endpoint for monitoring.
			- #### Logging Format
				- The log format follows the Bunyan specification with these required fields:
					- `v`: Integer. Bunyan log format version (currently 0).
					- `name`: String. Logger name.
					- `hostname`: String. Hostname of the machine.
					- `pid`: Integer. Process ID.
					- `level`: Integer. Log level (10=TRACE, 20=DEBUG, 30=INFO, 40=WARN, 50=ERROR, 60=FATAL).
					- `time`: String. ISO 8601 timestamp in UTC.
					- `msg`: String. Log message.
				- Optional Fields:
					- `src`: Object. Source code location info (not recommended for production).
					- `err`: Object. Error object with stack trace.
					- `req`: Object. HTTP request details.
					- `res`: Object. HTTP response details.
				- JSON Format Example:
					```json
					{
					  "v": 0,
					  "name": "weblogproxy",
					  "hostname": "server1",
					  "pid": 12345,
					  "level": 30,
					  "time": "2024-03-14T12:34:56.789Z",
					  "msg": "Processing request",
					  "site_id": "site1",
					  "user_id": "user123"
					}
					```
				- Text Format Example:
					```
					[2024-03-14T12:34:56.789Z] INFO: Processing request (site_id=site1, user_id=user123)
					```
				- For viewing and analyzing log files, we recommend using [lnav](https://lnav.org/), a powerful log file navigator.
		- ### Rule Processing Logic
			- The rule processor follows these key principles:
				1. Rules are processed in order from first to last
				2. Disabled rules are completely skipped
				3. Rules with `Continue: true`:
				   - Only accumulate values (scripts, addLogData)
				   - Do NOT affect logging decision
				   - Do NOT affect target destinations
				4. Rules with `Continue: false` (or not set):
				   - If enabled and matching, determine final logging decision
				   - Set target destinations
				   - Stop further rule processing
				5. If no enabled non-continue rule matches:
				   - Logging is disabled (`ShouldLogToServer: false`)
				   - No target destinations are set
				   - Accumulated values from continue rules are still available
		- ### Security Characteristics
			- Dynamic security token generation with configurable expiration.
			- Rule-based filtering (`condition` in `log_config`).
			- Configurable request size and rate limits (`server.request_limits`).
			- Logging of unauthorized access attempts.
			- Input sanitization and validation for all endpoints.
			- Token-based Authentication: Each client receives a signed token via `/logger.js`, which must be included in log requests.
			- Error Handling & Logging: All errors are logged with context for audit and debugging.
			- Separation of Destinations: Logs can be routed to different destinations, isolating sensitive data if needed.
		- ### Containerization
			- Minimal Alpine Linux base image.
			- Multi-stage build.
			- Configuration via YAML config bound to the container.
		- ### Operational Requirements
			- Two modes: `standalone` (own hostname, requires CORS) or `embedded` (URL hierarchy, uses `path_prefix`).
			- External hostname configuration support (implicitly via reverse proxy).
			- CORS configuration (`server.cors`).
			- Definable location (`server.path_prefix`).
			- Easy scalability.
			- Minimal system resource consumption.
			- Graceful shutdown for handling termination signals properly.
		- ### Monitoring and Diagnostics
			- Internal logging of critical events.
			- Basic health check endpoint.
		- ### Testing Strategy
			- #### Unit Tests
				- Comprehensive test coverage for core components:
					- Config Parser and Validator
					- Rule Processor with rule matching logic
					- Token Manager (generation and validation)
					- Logger Interface implementations
					- Data Enricher/Merger
					- IP Utilities for client IP determination
					- Rate Limiter middleware
				- Test cases for destination-specific formatting (Bunyan vs. GELF).
				- Configuration validation tests.
				- Run unit tests with: `go test ./...`
			- #### Integration Tests
				- End-to-end testing of API endpoints (`/logger.js`, `/log`, `/health`).
				- Testing different configuration scenarios (rule matching, disabled rules, data enrichment/merging, script injection).
				- Testing output formats for different destinations.
				- Testing log rotation functionality.
				- Security testing (token validation, request limits).
			- #### End-to-End (E2E) Tests
				- Uses [Hurl](https://hurl.dev/) to interact with a running instance of the application.
				- Verifies behavior of the `/logger.js` and `/log` endpoints based on the `config/test.yaml` configuration.
				- Run E2E tests with: `./test/run.sh [optional_filter_name_pattern]`
				- The testing script:
					1. Builds the application
					2. Starts the `weblogproxy` server in the background using `config/test.yaml`
					3. Executes the Hurl tests (`*.hurl`) found in `test/api/`
					4. Executes corresponding Bash scripts (`*.sh`) for validation if they exist
					5. Stops the server
					6. Reports overall success or failure
		- ### Architecture Overview
			```mermaid
			graph TD
			  Client[Web Client / Browser]
			  Client -->|/logger.js| WebLogProxy[WebLogProxy Server]
			  Client -->|/log| WebLogProxy
			  WebLogProxy -->|Log Forwarding| Destinations[Log Destinations]
			  Destinations -->|File, Syslog, GELF| Storage[Storage/External Systems]
			```

			- **Web Client**: Loads logger.js and sends logs via HTTP.
			- **WebLogProxy Server**: Handles log ingestion, rule processing, enrichment, security, and forwards logs.
			- **Log Destinations**: Configurable outputs (file, syslog, GELF, etc.).
		- ### Development & Versioning
			- #### Mise Tasks
				This project uses [Mise](https://mise.jdx.dev/) for task automation. Below is a list of key tasks:
				
				| Task Name | Description |
				|-----------|-------------|
				| build | Build the weblogproxy binary executable |
				| test | Run all tests |
				| lint | Run Go linters |
				| security-check | Run all security checks |
				| docker-build | Build the Docker image |
				| docker-test | Test Docker image and functionality |
				| publish | Publish a new version |
				| version-bump-dev | Set version to -dev and prepare [Unreleased] changelog section |
				| version-bump-release | Bump version for release and update changelog |
				
			- #### Versioning Process
				WebLogProxy uses a two-phase versioning process:

				- **Development version:**
				  - Run `mise run version-bump-dev`.
				  - Sets the version with the `-dev` suffix (e.g., `1.2.3-dev`) in `internal/version/version.go`.
				  - Prepares the `[Unreleased]` section in `CHANGELOG.md`.
				  - All builds and Docker images will be marked as dev.

				- **Release version:**
				  - Run `mise run version-bump-release -- -y patch` (or `minor`/`major`).
				  - Removes the `-dev` suffix from the version, moves the contents of `[Unreleased]` to a new section with the version number and date in the changelog.
				  - All builds and Docker images will be marked as release.
	- ## Project Structure
		- /cmd - Application entry points (weblogproxy main app, config-validator)
		- /internal - Private application code (including core components like Rule Processor)
			- /config - Configuration handling
			- /enricher - Data enrichment functionality
			- /handler - HTTP handlers for endpoints
			- /iputil - IP and CIDR handling utilities
			- /logger - Logger interfaces and implementations
			- /rules - Rule processing logic
			- /security - Security related functionality
			- /server - Server implementation
			- /truncate - Log data truncation utilities
			- /validation - Input validation functions
			- /version - Version information
		- /config - Configuration examples (test.yaml, example.yaml)
		- /test - Test files and mocks
			- /api - E2E test files using Hurl
		- /scripts - Utility scripts for the project
			- `config.sh`: Loads version and repository information from VCS
			- `bump-version.sh`: Increments the version in codebase
			- `publish.sh`: Creates a release and pushes to GitHub and GHCR
			- `docker-test.sh`: Tests Docker image build and functionality
			- `docker-ssh-copy.sh`: Deploys Docker image to a remote server via SSH
	- ## Installation & Quick Start
		- ### Prerequisites
			- Go (version 1.21 or later recommended)
			  OR
			- Docker (for containerized deployment)
			- Hurl (for running E2E tests)
		- ### Build & Run
			```bash
			# Build the binary
			go build -o weblogproxy ./cmd/weblogproxy
			
			# Copy and configure
			cp config/example.yaml config/config.yaml
			# Edit config/config.yaml according to your needs
			
			# Run
			./weblogproxy --config config/config.yaml
			
			# Validate config without running
			./weblogproxy --config /path/to/your/config.yaml -validate
			```
		- ### Using Docker
			```bash
			docker build -t weblogproxy:latest .
			docker run -p 8080:8080 -v $(pwd)/config:/app/config weblogproxy:latest
			```
		
		- ### Docker Usage with Custom UID/GID
			By default, the Docker container runs with a non-root user with UID/GID 1000. You can customize this by setting environment variables:

			```bash
			# Run with specific UID/GID (replace 1001/1001 with your values)
			docker run -p 8080:8080 -e PUID=1001 -e PGID=1001 weblogproxy:latest

			# Run as the current user
			docker run -p 8080:8080 -e PUID=$(id -u) -e PGID=$(id -g) weblogproxy:latest
			```

			#### Using docker-compose
			```yaml
			environment:
			  - PUID=1001
			  - PGID=1001
			```

			This is useful for:
			- Matching permissions with the host user for mounted volumes
			- Running the application with the same permissions as the host user
			- Ensuring logs and config files are owned by the appropriate user
