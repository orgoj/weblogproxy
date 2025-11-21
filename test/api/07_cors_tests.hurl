# ---------------------------------------------
# CORS Tests - Testing CORS middleware functionality
# ---------------------------------------------

# Test 7.1: Preflight OPTIONS request - origin is allowed (wildcard)
OPTIONS {{log_url}}
Origin: http://localhost:3000
Access-Control-Request-Method: POST
Access-Control-Request-Headers: Content-Type, Authorization
HTTP 204
[Asserts]
    header "Access-Control-Allow-Origin" == "*"
    header "Access-Control-Allow-Methods" == "GET, POST, OPTIONS"
    header "Access-Control-Allow-Headers" contains "Content-Type"
    header "Access-Control-Allow-Headers" contains "Authorization"
    # NOTE: Access-Control-Allow-Credentials is NOT set with wildcard origin (CORS spec)
    header "Access-Control-Max-Age" exists
    header "Access-Control-Max-Age" == "3600"

# Test 7.2: Preflight OPTIONS request with disallowed Origin
OPTIONS {{log_url}}
Origin: https://evil-site.com
Access-Control-Request-Method: POST
Access-Control-Request-Headers: Content-Type
HTTP 204
[Asserts]
    header "Access-Control-Allow-Origin" == "*"
    header "Access-Control-Allow-Methods" == "GET, POST, OPTIONS"
    header "Access-Control-Allow-Headers" contains "Content-Type"
    # NOTE: Access-Control-Allow-Credentials is NOT set with wildcard origin (CORS spec)

# Test 7.3: Preflight OPTIONS request for explicitly allowed Origin
OPTIONS {{log_url}}
Origin: https://test.example.com
Access-Control-Request-Method: POST
Access-Control-Request-Headers: Content-Type
HTTP 204
[Asserts]
    header "Access-Control-Allow-Origin" == "*"
    header "Access-Control-Allow-Methods" == "GET, POST, OPTIONS"
    header "Access-Control-Allow-Headers" contains "Content-Type"
    # NOTE: Access-Control-Allow-Credentials is NOT set with wildcard origin (CORS spec)

# Test 7.4: Actual POST request with allowed Origin
# Get valid token
GET {{logger_js_url}}?site_id={{valid_site_id}}&gtm_id={{valid_gtm_id}}
HTTP 200
[Captures]
    token: body regex "token: \"(\\d+:[a-f0-9]+)\""

# Use token in POST request with allowed Origin
POST {{log_url}}
Origin: https://test.example.com
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "CORS test",
        "level": "info",
        "test_run": "07_cors_tests.hurl"
    }
}
HTTP 200
[Asserts]
    header "Access-Control-Allow-Origin" == "*"
    # NOTE: Access-Control-Allow-Credentials is NOT set with wildcard origin (CORS spec)
    header "X-Log-Status" == "success"

# Test 7.5: POST request with "dangerous" Origin
# (but in wildcard mode it works as well)
# Get valid token again
GET {{logger_js_url}}?site_id={{valid_site_id}}&gtm_id={{valid_gtm_id}}
HTTP 200
[Captures]
    token: body regex "token: \"(\\d+:[a-f0-9]+)\""

# Use token in POST
POST {{log_url}}
Origin: https://evil-site.com
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "CORS test with arbitrary origin",
        "level": "info",
        "test_run": "07_cors_tests.hurl"
    }
}
HTTP 200
[Asserts]
    header "Access-Control-Allow-Origin" == "*"
    # NOTE: Access-Control-Allow-Credentials is NOT set with wildcard origin (CORS spec)
    header "X-Log-Status" == "success" # Works in wildcard mode 