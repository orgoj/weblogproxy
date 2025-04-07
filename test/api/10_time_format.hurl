# ---------------------------------------------
# Time Format Test
# ---------------------------------------------

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

# Send a test log entry with timestamp test marker
POST {{log_url}}
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Time format test",
        "test_id": "time_format_test",
        "timestamp_test": true
    }
}
HTTP 200
[Asserts]
    status == 200
    body == ""
    header "X-Log-Status" == "success" 