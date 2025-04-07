# ------------------------------------------------------
# /logger.js Endpoint Tests - Invalid Scenarios
# ------------------------------------------------------

# Test 1.4: Get logger.js with missing site_id
GET {{logger_js_url}}?gtm_id={{valid_gtm_id}}
[Options]
    # insecure: true # Assume set globally
HTTP 200
[Asserts]
    header "Content-Type" contains "application/javascript"
    body contains "logEnabled: false,"
    body contains "token: \"\""
    body contains "logUrl: \"\""
    body contains "scriptsToInject: ["
    body contains "]"
    body contains "window.weblogproxy"

# Test 1.5: Get logger.js with invalid site_id format
GET {{logger_js_url}}?site_id=invalid!site?id
[Options]
    # insecure: true # Assume set globally
HTTP 200
[Asserts]
    header "Content-Type" contains "application/javascript"
    body contains "logEnabled: false,"
    body contains "token: \"\""
    body contains "logUrl: \"\""
    body contains "scriptsToInject: ["
    body contains "]"
    body contains "window.weblogproxy"

# Test 1.6: Get logger.js with invalid gtm_id format
GET {{logger_js_url}}?site_id={{valid_site_id}}&gtm_id=invalid!gtm?id
[Options]
    # insecure: true # Assume set globally
HTTP 200
[Asserts]
    header "Content-Type" contains "application/javascript"
    body contains "logEnabled: false,"
    body contains "token: \"\""
    body contains "logUrl: \"\""
    body contains "scriptsToInject: ["
    body contains "]"
    body contains "window.weblogproxy" 