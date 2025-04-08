# WebLogProxy

![Version](https://img.shields.io/github/v/tag/orgoj/weblogproxy?label=version&sort=semver)
![CI Status](https://github.com/orgoj/weblogproxy/actions/workflows/ci.yml/badge.svg)
![License](https://img.shields.io/github/license/orgoj/weblogproxy)
![Go Version](https://img.shields.io/github/go-mod/go-version/orgoj/weblogproxy)

**Current version: 0.9.0** - See [CHANGELOG.md](CHANGELOG.md) for details

## Overview

WebLogProxy is a flexible and performant web log processor designed to receive logs from client-side JavaScript, enrich them with server-side information, apply processing rules, and forward them to various destinations like files (with rotation) or GELF endpoints.

## Features

*   **Dynamic Client-Side Logger:** `/logger.js` endpoint generates a JavaScript logger tailored to the requesting client based on rules.
*   **Rule-Based Processing:** Define rules in YAML to control logging behavior, data enrichment, and script injection based on Site ID, GTM ID, User Agent, and IP address.
*   **Data Enrichment:** Automatically add server timestamp, client IP, User Agent, Referer, and selected headers/query params to log records.
*   **Multiple Destinations:** Send logs to files (with rotation and compression options) or GELF endpoints.
*   **Security:** Uses time-limited tokens to prevent unauthorized log submissions.
*   **Performance:** Designed with performance in mind, using Go templates and efficient processing.
*   **Flexibility:** Run in `standalone` mode or `embedded` within an existing site structure.
*   **CORS Support:** Configurable Cross-Origin Resource Sharing headers.
*   **Graceful Shutdown:** Handles termination signals properly.
*   **Containerized:** Includes a multi-stage Dockerfile based on Alpine Linux.

## Getting Started

### Prerequisites

*   Docker (for containerized deployment)

### Quick Start with Docker

#### 1. Create a config and logs directory

Create a `config` directory and copy the `config/docker-config.yaml` file to it. Edit it to your needs.

Create a `logs` directory to store the log files.


#### 2a.  Using Docker CLI

Run the following command to start the container:

```bash
docker run -p 8080:8080 -v $(pwd)/config:/app/config -v $(pwd)/logs:/app/logs weblogproxy:latest
```

#### 2b.  Using Docker Compose

Create a `docker-compose.yml` file:

```yaml
version: '3.8'

services:
    weblogproxy:
    image: weblogproxy:latest
    container_name: weblogproxy
    restart: unless-stopped
    ports:
        - "8080:8080"
    volumes:
        - ./config:/app/config
        - ./logs:/app/logs
    environment:
        - TZ=Europe/Prague
```

Then run:

```bash
docker compose up -d
```


### Configuration

Configuration is done via a YAML file. See `config` for examples.

### Key Sections:

*   `server`: Host, port, mode (`standalone`/`embedded`), `domain` (for standalone mode), `path_prefix` (for embedded), CORS settings, custom headers for `/logger.js`, request limits (body size, rate limit), trusted proxies.
    *   CORS settings include:
        *   `enabled`: Turn CORS support on/off
        *   `allowed_origins`: Array of allowed origins (must be complete URLs starting with http:// or https://)
        *   `max_age`: Cache duration for preflight requests in seconds
    *   Note: Using wildcard "*" in allowed_origins is supported but not recommended for production environments
*   `security`: Token secret and expiration duration.
*   `log_destinations`: Define named outputs (type: `file`, `gelf`).
    *   `file`: `path`, `format` (`json`/`text`), `rotation` (`max_size` in MB - integer value with minimum 1MB due to Lumberjack library limitations, `max_age` in Lumberjack duration format, `max_backups`, `compress`).
    *   `gelf`: `host`, `port`, `protocol`, `compression_type`.
    *   `add_log_data`: Destination-specific data enrichment.
*   `log_config`: An ordered list of rules controlling processing.
    *   `condition`: Matching conditions including:
        *   `site_id`: Match by site identifier
        *   `gtm_ids`: Match by Google Tag Manager IDs
        *   `user_agents`: Match by user agent patterns (supports glob)
        *   `ips`: Match by client IP addresses/CIDRs
        *   `headers`: Match by HTTP request headers. Supports three value types:
            *   String value: Header must exist with exactly this value
            *   Boolean `true`: Header must exist (any value)
            *   Boolean `false`: Header must NOT exist
    *   `add_log_data`: Enrichment data to add to log records. Supports a special `value: "false"` to remove previously defined fields:
        *   To remove a field previously added by a rule or destination, use `{name: "field_name", source: "static", value: "false"}`

## URL Behavior

WebLogProxy supports two operational modes that affect URL generation:

* **Embedded Mode**: When running in embedded mode (`server.mode: "embedded"`), the application generates relative URLs for internal endpoints. This mode requires setting a `server.path_prefix` value to specify the URL path where the application is mounted.

* **Standalone Mode**: When running in standalone mode (`server.mode: "standalone"`), the application generates absolute URLs that include protocol and domain. This mode requires setting a `server.domain` value to specify the domain name used to construct the absolute URLs.

## JavaScript Logger Behavior

The `/logger.js` endpoint generates JavaScript code with the following characteristics:

* **When Logging is Enabled**: Returns a full-featured JavaScript that includes a token, configured log URL, and all necessary functionality for logging and script injection.

* **When Logging is Disabled**: Returns the same JavaScript structure but with `logEnabled: false`, empty token, and empty log URL. The log function becomes a no-op function that silently ignores all calls.

* **Script Injection**: Always includes the scripts configured in matching rules, even when logging is disabled. This allows for injecting tracking or other scripts independently of the logging functionality.

The logger operates invisibly in the browser with no console output, ensuring quiet operation even in production environments.

## HTML Usage Example

To use WebLogProxy in your HTML, include the following script and call the `window.weblogproxy.log` function to log events:

```html
<script src="https://yourdomain.com/logger.js?site_id=example.com"></script>
<script>
    // Example log event
    window.weblogproxy.log({
        event: 'button_click',
        buttonId: 'logButton',
    });
</script>
```

In this example, the `logger.js` script is included, and the `window.weblogproxy.log` function is called to log an event. The event data is sent to the WebLogProxy server.

## Log File Format

The log format follows the [Bunyan](https://github.com/trentm/node-bunyan) specification. Bunyan is a simple and fast JSON logging library for Node.js services.

### Core Fields

Every log record is a JSON object with the following required fields:

- `"v"`: Integer. Bunyan log format version (currently 0). Added by Bunyan, cannot be overridden.
- `"name"`: String. Logger name. Must be specified when creating the logger.
- `"hostname"`: String. Hostname of the machine. Retrieved via os.hostname() if not specified.
- `"pid"`: Integer. Process ID. Filled in automatically.
- `"level"`: Integer. Log level (see below). Added by Bunyan, cannot be overridden.
- `"time"`: String. ISO 8601 timestamp in UTC. Added by Bunyan, can be overridden.
- `"msg"`: String. Log message. Required for every log call.

### Log Levels

Bunyan uses the following numeric log levels:

- `10`: TRACE
- `20`: DEBUG
- `30`: INFO
- `40`: WARN
- `50`: ERROR
- `60`: FATAL

### Optional Fields

- `"src"`: Object. Source code location info. Added automatically if "src: true" is configured. Not recommended for production use.
- `"err"`: Object. Error object with stack trace.
- `"req"`: Object. HTTP request details.
- `"res"`: Object. HTTP response details.

### Example JSON Format

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

### Text Format

For human-readable logs, the text format follows this pattern:

```
[2024-03-14T12:34:56.789Z] INFO: Processing request (site_id=site1, user_id=user123)
```

For viewing and analyzing log files, we recommend using [lnav](https://lnav.org/), a powerful log file navigator that provides:
- Syntax highlighting for JSON
- Automatic log format detection
- SQL-like querying capabilities
- Timeline view
- Search and filtering

Example usage with lnav:
```bash
# View logs in real-time
lnav -f /path/to/logs/*.log

# Search for specific events
lnav -c ':filter-in msg =~ "error"'

# Query logs using SQL
lnav -c ':sql SELECT time, msg FROM logs WHERE level >= 50'
```

## Rule Processing Logic

The rule processor follows these key principles:

1. Rules are processed in order from first to last
2. Disabled rules are completely skipped
3. Rules with `continue: true`:
   - Only accumulate values (scripts, addLogData)
   - Do NOT affect logging decision
   - Do NOT affect target destinations
4. Rules with `continue: false` (or not set):
   - If enabled and matching, determine final logging decision
   - Set target destinations
   - Stop further rule processing
5. If no enabled non-continue rule matches:
   - Logging is disabled (`ShouldLogToServer: false`)
   - No target destinations are set
   - Accumulated values from continue rules are still available

## Development

### Required Tools

* **Go**: For building the application from source.
* **Docker**: For containerized deployment.
* **Hurl**: For end-to-end testing.
* **Mise**: For deployment.

### Building the Application

If you prefer to build the application from source, follow these steps:

1.  Build the application:
    ```bash
    go build -o weblogproxy ./cmd/weblogproxy
    ```

2.  Copy and configure:
    ```bash
    cp config/example.yaml config/config.yaml
    # Edit config/config.yaml according to your needs
    ```

3.  Run:
    ```bash
    ./weblogproxy --config config/config.yaml
    ```

### Development Deployment via SSH

For development and testing purposes, you can deploy the Docker image directly to a remote server via SSH without publishing to a registry:

```bash
# Deploy to remote server (runs docker-test first)
./scripts/docker-ssh-copy.sh user@remote-server [additional_ssh_options]

# Alternative using mise
mise run docker-ssh-copy -- user@remote-server [additional_ssh_options]
```

This will:
1. Tag the existing test image with the current version and timestamp (e.g., `0.9.0-20230615-120530`)
2. Transfer the image to the remote server via SSH
3. Clean up the local tagged image

The timestamp in the image tag allows you to easily identify when each image was built when running `docker ps` on the remote server.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Origin of the Program

WebLogProxy was created because I needed to log events from Google Tag 
Manager. I developed the entire project using the Cursor IDE, without 
any prior experience in the Go programming language. I didn't write a 
single line of code and was just learning to use the editor.

