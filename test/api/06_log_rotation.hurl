# ----------------------------------------------------
# /log Endpoint Test - Log Rotation Setup
# ----------------------------------------------------
# This Hurl file simply sends ONE log entry to ensure the log file exists
# and that the token is valid before the .sh script takes over to generate load.
# The actual load generation and rotation validation happens in 06_log_rotation.sh

# Get logger.js to obtain token
GET {{logger_js_url}}?site_id={{valid_site_id}}&gtm_id={{valid_gtm_id}}
[Options]
    retry: 10
    retry-interval: 1000
HTTP 200
[Captures]
    token: body regex "token: \"(\\d+:[a-f0-9]+)\""
[Asserts]
    status == 200
    header "Content-Type" contains "application/javascript"
    body contains "token: \""
    body contains ":"
    body contains "\","
    body contains "logEnabled: true,"

# ----------------------------------------------------
# /log Endpoint Test - Log Rotation Trigger
# ----------------------------------------------------
# This Hurl file sends a large number of log entries using `repeat: N`
# to trigger file rotation.
# The validation of rotated files happens in 06_log_rotation.sh

POST {{log_url}}
[Options]
repeat: 4200
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Rotation test log entry",
        "level": "debug",
        "padding": "XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
        "test_run": "06_log_rotation"
    }
}

HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"
