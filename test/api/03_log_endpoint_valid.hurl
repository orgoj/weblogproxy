# ---------------------------------------------
# /log Endpoint Tests - Valid Scenarios
# ---------------------------------------------

# Test 3.1: Get logger.js to obtain token
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

# Test 3.2: Send a valid log entry using captured token
POST {{log_url}}
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Valid log entry test from Hurl",
        "level": "info",
        "test_run": "03_log_endpoint_valid.hurl",
        "custom_field": 123
    }
}
HTTP 200
[Asserts]
    status == 200
    body == ""
    header "X-Log-Status" == "success"

# Check that request with invalid token fails
POST {{log_url}}
Content-Type: application/json
{
    "token": "invalid_token",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Invalid log entry test from Hurl",
        "level": "info",
        "test_run": "03_log_endpoint_valid.hurl",
        "custom_field": 123
    }
}
HTTP 200
[Asserts]
    status == 200
    body == ""
    header "X-Log-Status" == "failure"