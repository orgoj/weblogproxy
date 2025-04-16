# WebLogProxy

WebLogProxy is a flexible and secure web logging proxy that allows you to collect and forward client-side logs from web applications to various destinations.

## Features

* **Multiple Logging Destinations**: Configure multiple destinations for logs, including files, syslog servers, and GELF endpoints.
* **Rule-based Logging**: Define rules based on site ID, GTM ID, user agent, and client IP to control logging behavior.
* **Script Injection**: Inject scripts based on rules, even when logging is disabled.
* **Data Enrichment**: Add or modify log data with values from various sources (static, HTTP headers, query parameters, post data).
* **Security**: Secure token generation and validation with configurable expiration.
* **Rate Limiting**: Protect against abuse with configurable rate limits.
* **Flexible Deployment**: Run as a standalone server or embedded behind a reverse proxy.
* **CORS Support**: Configure CORS for cross-origin requests.
* **Minimal Footprint**: Built in Go for speed and efficiency, with a small memory footprint.

## Quick Start

### Using Docker

```bash
# Clone the repository
git clone https://github.com/orgoj/weblogproxy.git
cd weblogproxy

# Copy and edit the configuration file
cp config/example.yaml config/config.yaml
# Edit config/config.yaml to your needs

# Build and run with Docker
docker-compose up -d
```

### Building from Source

```bash
# Clone the repository
git clone https://github.com/orgoj/weblogproxy.git
cd weblogproxy

# Build the application
go build -o weblogproxy ./cmd/weblogproxy

# Copy and edit the configuration file
cp config/example.yaml config/config.yaml
# Edit config/config.yaml to your needs

# Run the application
./weblogproxy -config config/config.yaml
```

## Configuration

WebLogProxy is configured through a YAML configuration file. The file is divided into sections that control the server, security settings, log destinations, and log rules.

Create a new configuration file based on the example:

```bash
cp config/example.yaml config/config.yaml
```

### Server Configuration

```yaml
server:
  host: "0.0.0.0"  # Listen on all interfaces
  port: 8080
  mode: "standalone"  # standalone or embedded
  domain: "log.example.com"  # Required for standalone mode
  path_prefix: ""  # Required for embedded mode
  # Add other server settings as needed (CORS, headers, etc.)
```

## API Endpoints

The server provides the following endpoints:

* **GET /logger.js**: Returns a JavaScript for client-side logging. Requires `site_id` parameter, optional `gtm_id`.
* **POST /log**: Receives log data from the client. Requires a valid token from /logger.js.
* **GET /health**: Simple health check endpoint.

## /logger.js Endpoint

The `/logger.js` endpoint is the entry point for the logging system. It returns a JavaScript file that sets up the logging infrastructure in the client's browser. The returned JavaScript has two possible behaviors:

* **When Logging is Enabled**: Returns a full-featured JavaScript that includes a token, configured log URL, and all necessary functionality for logging and script injection.

* **When Logging is Disabled**: Returns a JavaScript with a no-op function that silently ignores all calls, without any configuration object.

* **Script Injection**: Always includes the scripts configured in matching rules, even when logging is disabled. This allows for injecting tracking or other scripts independently of the logging functionality.

The logger operates invisibly in the browser with no console output, ensuring quiet operation even in production environments.

## HTML Usage Example

To use WebLogProxy in your HTML, include the following script and call the `window.wlp.log` function to log events:

```html
<script src="https://yourdomain.com/logger.js?site_id=example.com"></script>
<script>
    // Example log event
    window.wlp.log({
        event: 'button_click',
        buttonId: 'logButton',
    });
</script>
```

In this example, the `logger.js` script is included, and the `window.wlp.log` function is called to log an event. The event data is sent to the WebLogProxy server.

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
lnav -f /path/to/log/*.log

# Search for specific events
lnav -c ':filter-in msg =~ "error"'

# Query logs using SQL
lnav -c ':sql SELECT time, msg FROM log WHERE level >= 50'
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

