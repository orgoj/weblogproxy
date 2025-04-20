# PRD: WebLogProxy

## 1. Product overview

### 1.1 Document title and version
- PRD: WebLogProxy  
- Version: 0.11.0

### 1.2 Product summary
WebLogProxy is a flexible and secure web logging proxy built in Go. It enables client‑side JavaScript applications to send structured log events to a central service, which then forwards these events to multiple destinations (files, syslog, GELF).

The tool supports rule‑based logging—filtering or enriching data based on site ID, Google Tag Manager ID, user agent, client IP or CIDR blocks—and allows script injection independently of logging. Security features include token generation/validation, rate limiting, CORS configuration, and an HTTP health check endpoint.

With both standalone and embedded modes, WebLogProxy integrates seamlessly into existing deployments, offering minimal footprint and high performance. It is ideal for teams seeking lightweight, configurable, and secure client‑side log collection.

## 2. Goals

### 2.1 Business goals
- Reduce the time to diagnose front‑end issues by centralizing client‑side logs.  
- Offer an extensible logging solution for customers using GTM or custom scripts.  
- Increase adoption by providing Docker‑based deployment and clear documentation.  
- Minimize support costs through robust testing and self‑service health/version endpoints.

### 2.2 User goals
- Quickly embed a single `<script>` to start logging client events.  
- Define fine‑grained logging rules (by site, IP, user agent).  
- Secure log ingestion with expiring tokens and optional CORS restrictions.  
- Forward logs to multiple backends without changing application code.  
- Monitor service health and version easily via HTTP endpoints.

### 2.3 Non‑goals
- Not providing a UI for viewing or analyzing logs.  
- Not storing logs long‑term or managing retention policies.  
- Not performing complex analytics or visualizations.  
- Not acting as a full APM or error‑tracking solution.  

## 3. User personas

### 3.1 Key user types
- Developer  
- DevOps engineer  
- Security analyst  
- Site owner

### 3.2 Basic persona details
- **Developer**: Integrates `logger.js` into web pages and calls `window.wlp.log(...)` for event tracking.  
- **DevOps engineer**: Deploys and configures WebLogProxy (Docker or binary), sets up rules and destinations.  
- **Security analyst**: Reviews logs for anomalies, relies on token‑based auth and rate limits.  
- **Site owner**: Ensures third‑party scripts are injected even when logging is disabled.

### 3.3 Role‑based access
- **Admin**: Full control over configuration (rules, destinations, server settings), can view health and version endpoints.  
- **Client**: Browser script retrieving `/logger.js`, obtaining a token, and sending logs to `/log`.  
- **Monitoring system**: Polls `/health` and `/version` endpoints for service status.

## 4. Functional requirements
- **Retrieve logging script** (Priority: High)  
  - Serve `/logger.js?site_id={site}` returning JavaScript with token, config, and injection list.  
  - Support optional `gtm_id` parameter.  
- **Send log data** (Priority: High)  
  - Accept `POST /log` with valid token, respond 200 on success.  
  - Reject invalid/expired tokens (401) and rate‑limit violations (429).  
- **Rule‑based filtering and enrichment** (Priority: High)  
  - Load rules from YAML: match on site ID, GTM ID, UserAgent, IP/CIDR, headers.  
  - Support `continue` flag, data/script accumulation, and disabled rules.  
- **Multiple destinations** (Priority: High)  
  - Forward logs to file, syslog, and GELF backends concurrently.  
  - Support UDP/TCP and configurable compression for GELF.  
- **Rate limiting and body size** (Priority: High)  
  - Enforce `rate_limit` (requests/minute per IP) and `max_body_size` settings.  
- **Security and CORS** (Priority: Medium)  
  - Generate and validate JWT tokens with expiration.  
  - Configure CORS origins and headers.  
- **Health and version endpoints** (Priority: Medium)  
  - Expose `/health` (200/ok) and `/version` (returns build version).  
  - Restrict `/health` by CIDR‑based `health_allowed_ips`.  
- **Unknown route handling** (Priority: Low)  
  - Return configured HTTP status code and cache header for undefined paths.  
- **Standalone and embedded modes** (Priority: Low)  
  - Standalone: serve on its own domain.  
  - Embedded: mount under a path prefix behind a reverse proxy.

## 5. User experience

### 5.1 Entry points & first‑time user flow
- Developer or DevOps engineer installs via Docker Compose or binary.  
- Copies `config/example.yaml` to `config/config.yaml` and customizes rules/destinations.  
- Starts WebLogProxy (`docker compose up` or `./weblogproxy --config config/config.yaml`).  
- Embeds `<script src="https://domain/logger.js?site_id=example.com"></script>` in HTML.  
- Calls `window.wlp.log({...})` in application code to generate client‑side events.

### 5.2 Core experience
- **Embed logger script**: Developer inserts `/logger.js` with `site_id`.  
  - Script loads quickly; displays no console errors.  
- **Obtain token**: Script fetches token transparently on page load.  
  - Token retrieval errors are retried with exponential backoff.  
- **Log events**: Calls to `window.wlp.log` send HTTP POSTs to `/log`.  
  - Payloads under size limit; errors surfaced to console if debug enabled.  
- **Forward to backends**: Server writes to file and/or sends to GELF/syslog.  
  - Backends confirm receipt via synchronous or buffered writes.  
- **Monitor status**: Ops team polls `/health` and `/version`.  
  - Dashboard shows "ok" health and correct version.

### 5.3 Advanced features & edge cases
- Script injection occurs even when logging disabled.  
- Unknown routes return configurable status and cache headers.  
- CORS preflight requests honored or rejected per config.  
- JWT token expiration and refresh.  
- Large payloads and rate‑limit boundary conditions.

### 5.4 UI/UX highlights
- Zero‑dependency client script; silent by default.  
- Clear JSON error messages for clients (401, 429, 413).  
- Fast startup (<100 ms) and low memory footprint.  
- Self‑documenting config with example YAML.

## 6. Narrative
An operations engineer, Jana, needs to collect client‑side events from multiple websites without building a custom backend. She deploys WebLogProxy via Docker, defines rules for her various domains, and embeds the provided `logger.js` script in her pages. Jana sees logs forwarded instantly to her Graylog instance and files, and uses the `/health` endpoint to integrate with monitoring—giving her confidence in real‑time observability.

## 7. Success metrics

### 7.1 User‑centric metrics
- Time to first log event ≤ 5 minutes from fresh install.  
- Developer satisfaction score ≥ 4/5 in surveys.  
- Percentage of successful script loads ≥ 99.9%.

### 7.2 Business metrics
- Number of active deployments per month.  
- Support tickets related to setup ≤ 5 per quarter.  
- Conversion rate for premium support subscriptions.

### 7.3 Technical metrics
- 95th‑percentile request latency to `/log` < 10 ms.  
- Error rate (4xx/5xx) < 0.1%.  
- Automated test coverage ≥ 90%.  
- Memory usage < 50 MB under 1 k RPS load.

## 8. Technical considerations

### 8.1 Integration points
- Docker Compose and Kubernetes deployments.  
- Google Tag Manager custom HTML tag.  
- Syslog servers and Graylog/GELF endpoints.  
- Existing monitoring (Prometheus, health checks).

### 8.2 Data storage & privacy
- Logs treated as structured JSON; sensitive fields sanitized in config.  
- No long‑term storage; intended for real‑time forwarding.  
- CORS and token auth prevent unauthorized ingestion.

### 8.3 Scalability & performance
- Support 1 k–5 k RPS with rate limiting.  
- Non‑blocking I/O for backend writes.  
- Configurable buffer sizes for GELF.

### 8.4 Potential challenges
- Evaluating large rule sets with minimal latency.  
- Handling bursts near rate limits.  
- Ensuring reliable compression and UDP packet delivery for GELF.  
- Coordinating config changes without service downtime.

## 9. Milestones & sequencing

### 9.1 Project estimate
- Medium: 2–4 weeks

### 9.2 Team size & composition
- Medium team (2–4 people)  
  - Product manager  
  - 1–2 Go engineers  
  - 1 QA/test engineer  
  - 1 DevOps engineer

### 9.3 Suggested phases
- **Phase 1:** Core logging endpoints and client script (2 weeks)  
  - Deliver `/logger.js`, `/log`, token auth, rule loading.  
- **Phase 2:** Destinations and enrichment (1 week)  
  - File, syslog, GELF forwarding; data enrichment and script injection.  
- **Phase 3:** Security, rate limiting, health/version (1 week)  
  - JWT logic, CORS, rate limits, `/health`, `/version`, tests, and docs.

## 10. User stories

### 10.1. Retrieve logging script
- **ID**: US‑001  
- **Description**: As a developer, I want to GET `/logger.js?site_id={site}` so that I can embed the client logging library in my web page.  
- **Acceptance criteria**:  
  - A valid request returns JavaScript containing a token, endpoint URL, and injection scripts.  
  - Missing or invalid `site_id` returns HTTP 400 with descriptive error.  
  - Logging disabled still returns injection scripts and a no‑op logger function.

### 10.2. Send log data
- **ID**: US‑002  
- **Description**: As a client, I want to POST log events to `/log` with a valid token so that the server records client‑side actions.  
- **Acceptance criteria**:  
  - Valid token and payload return HTTP 200.  
  - Invalid or expired token returns HTTP 401.  
  - Exceeding rate limit returns HTTP 429.  
  - Oversized payload (> max_body_size) returns HTTP 413.

### 10.3. Check service health
- **ID**: US‑003  
- **Description**: As an operator, I want to GET `/health` to verify the service is operational.  
- **Acceptance criteria**:  
  - Returns HTTP 200 and JSON `{"status":"ok"}`.  
  - Requests from disallowed IPs return HTTP 403.

### 10.4. Retrieve version
- **ID**: US‑004  
- **Description**: As an operator, I want to GET `/version` to confirm the deployed application version.  
- **Acceptance criteria**:  
  - Returns HTTP 200 and JSON `{"version":"0.11.0"}`.  
  - Value matches the build version.

### 10.5. Define logging rules
- **ID**: US‑005  
- **Description**: As an admin, I want to configure rule‑based logging in YAML so that events are filtered or enriched based on site, IP, or user agent.  
- **Acceptance criteria**:  
  - Configuration schema validates rules with site ID, GTM ID, CIDR, headers.  
  - Rules apply in defined order; `continue` flag respected.  
  - Disabled rules are ignored.

### 10.6. Inject scripts when logging disabled
- **ID**: US‑006  
- **Description**: As a site owner, I want scripts injected by rules to load even when logging is disabled so that tracking tags still operate.  
- **Acceptance criteria**:  
  - `/logger.js` returns a logger stub but includes all `script` rules.  
  - Injected script URLs match configuration.

### 10.7. Handle unknown routes
- **ID**: US‑007  
- **Description**: As a client, I want unknown paths to return configured status and cache headers so that requests to missing resources are handled gracefully.  
- **Acceptance criteria**:  
  - Requests to undefined endpoints return the `unknown_route.code`.  
  - Response includes `Cache-Control` header with `unknown_route.cache_control`.  
  - Body is empty.

### 10.8. Support multiple destinations
- **ID**: US‑008  
- **Description**: As an operator, I want logs forwarded to file, syslog, and GELF backends concurrently so that I can integrate with multiple logging systems.  
- **Acceptance criteria**:  
  - Log entries are written to file and sent to GELF endpoints as configured.  
  - GELF supports UDP, TCP, and compression settings.  
  - Failures in one destination do not block others.

### 10.9. Enforce rate limiting
- **ID**: US‑009  
- **Description**: As an operator, I want rate limits enforced on `/log` so that abusive clients are throttled.  
- **Acceptance criteria**:  
  - Requests per IP exceed `rate_limit` return HTTP 429.  
  - Rate limit reset intervals follow configuration.

### 10.10. Enrich log data
- **ID**: US‑010  
- **Description**: As an admin, I want to enrich incoming log events with HTTP headers and static values so that additional context is recorded.  
- **Acceptance criteria**:  
  - Enrichment rules in config add or overwrite fields in outgoing logs.  
  - Nested objects and arrays handled correctly according to truncation limits.
