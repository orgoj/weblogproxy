# ------------------------------------------------
# /log Endpoint Tests - Invalid Scenarios
# May require 'token' capture from previous file(s)
# ------------------------------------------------

# Test 2.2: Send log entry with missing required field (token)
POST {{log_url}}
Content-Type: application/json
[Options]
    # insecure: true # Assume set globally
{
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": { "message": "Missing token test from Hurl" }
}
HTTP 200 # Server currently accepts and logs error internally
[Asserts]
    status == 200
    body == ""

# Test 2.3: Send log entry with missing required field (site_id)
POST {{log_url}}
Content-Type: application/json
[Options]
    # insecure: true # Assume set globally
{
    "token": "{{token}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": { "message": "Missing site_id test from Hurl" }
}
HTTP 200 # Server currently accepts and logs error internally
[Asserts]
    status == 200
    body == ""

# Test 2.4: Send log entry with missing required field (data)
POST {{log_url}}
Content-Type: application/json
[Options]
    # insecure: true # Assume set globally
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}"
}
HTTP 200 # Server currently accepts and logs error internally
[Asserts]
    status == 200
    body == ""

# Test 2.5: Send log entry with invalid token
POST {{log_url}}
Content-Type: application/json
[Options]
    # insecure: true # Assume set globally
{
    "token": "{{invalid_token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": { "message": "Invalid token test from Hurl" }
}
HTTP 200 # Server currently accepts and logs error internally
[Asserts]
    status == 200
    body == ""

# Test 2.6 / Test 1.11 (Duplicate in original): Send log entry with invalid site_id format in body
POST {{log_url}}
Content-Type: application/json
[Options]
    # insecure: true # Assume set globally
{
    "token": "{{token}}",
    "site_id": "{{invalid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": { "message": "Invalid site_id in body test from Hurl" }
}
HTTP 200 # Server currently accepts and logs error internally
[Asserts]
    status == 200
    body == "" 