# ----------------------------------------------------
# /logger.js Endpoint Tests - Valid Scenarios
# ----------------------------------------------------

# Test 1.1: Get logger.js for a valid and enabled site/gtm ID
GET {{logger_js_url}}?site_id={{valid_site_id}}&gtm_id={{valid_gtm_id}}
[Options]
    # Shared options like insecure and retry can be in run.sh or here
    # insecure: true # Assume set globally in run.sh
    retry: 10         # Keep specific retries if needed per test
    retry-interval: 1000
HTTP 200
[Captures]
    token: body regex "token: \"(\\d+:[a-f0-9]+)\"" # Escaped regex
[Asserts]
    status == 200
    header "Content-Type" contains "application/javascript"
    header "Cache-Control" == "public, max-age=10, stale-while-revalidate=5"
    header "X-Test-Header" == "TestValue"
    body contains "token: \"" # Escaped quote
    body contains ":"
    body contains "\"," # Escaped quote and comma
    body contains "logEnabled: true,"
    body contains "logUrl: \"{{prefix}}/log\"" # Escaped quotes
    body contains "https://cdn.example.com/tracker.js"
    body contains "/local/scripts/init.js"
    body contains "/scripts/base-tracking.js"

# Test 1.2: Get logger.js with only site_id (GTM ID optional)
# Should only match Rule 0 (continue: true), no final rule matches.
# Expect LogEnabled=false, but base script injected.
# Expect Token="" and LogUrl=""
GET {{logger_js_url}}?site_id={{valid_site_id}}
[Options]
    # insecure: true # Assume set globally
HTTP 200
[Asserts]
    header "Content-Type" contains "application/javascript"
    body contains "logEnabled: false,"
    body contains "token: \"\"" # Empty token
    body contains "logUrl: \"\"" # Empty URL
    body contains "/scripts/base-tracking.js" # Base script from Rule 0
    body not contains "https://cdn.example.com/tracker.js" # Script from Rule 1
    body not contains "/local/scripts/init.js" # Script from Rule 1

# Test 1.3: Get logger.js for a disabled site (Rule 3 matches -> ShouldLogToServer=false, Rule 0 matches -> ShouldInjectScripts=true)
# Expect LogEnabled=false (server-side logging disabled), but base script injected
# Expect Token="" and LogUrl=""
GET {{logger_js_url}}?site_id={{disabled_site_id}}
[Options]
    # insecure: true # Assume set globally
    retry: 10         # Keep specific retries
    retry-interval: 1000
HTTP 200
[Asserts]
    header "Content-Type" contains "application/javascript"
    body contains "logEnabled: false,"
    body contains "token: \"\"" # Empty token
    body contains "logUrl: \"\"" # Empty URL
    body contains "/scripts/base-tracking.js" # Check for base script (due to ShouldInjectScripts=true)
    body not contains "https://cdn.example.com/tracker.js"
    body not contains "/local/scripts/init.js" 