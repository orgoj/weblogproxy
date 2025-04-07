# ------------------------------------------------------
# IP Detection and Trusted Proxies Test
# ------------------------------------------------------

# First get a token for valid site/GTM
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

# Test 1: Send normal request without X-Forwarded-For
# Should use the client's direct IP
POST {{log_url}}
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "IP detection test - direct IP",
        "test_id": "ip_detection_test_direct"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 2: Send request with X-Forwarded-For from a trusted proxy
# In test.yaml, 127.0.0.1 is configured as a trusted proxy
POST {{log_url}}
Content-Type: application/json
X-Forwarded-For: 192.168.100.100, 127.0.0.1
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "IP detection test - trusted proxy",
        "test_id": "ip_detection_test_trusted_proxy",
        "expected_ip": "192.168.100.100"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 3: Send request with X-Forwarded-For from an untrusted proxy
# An IP not in the trusted proxies list will be ignored
POST {{log_url}}
Content-Type: application/json
X-Forwarded-For: 192.168.99.99, 10.10.10.10
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "IP detection test - untrusted proxy",
        "test_id": "ip_detection_test_untrusted_proxy",
        "expected_ip": "127.0.0.1" # The actual client IP, not from XFF
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 4: Rule matching by IP from X-Forwarded-For with a trusted proxy
# Test rule 1 specifically includes 192.168.1.0/24 IPs
POST {{log_url}}
Content-Type: application/json
X-Forwarded-For: 192.168.1.123, 127.0.0.1
{
    "token": "{{token}}",
    "site_id": "{{valid_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "IP detection test - rule matching by IP from XFF",
        "test_id": "ip_detection_test_rule_match",
        "expected_rule_match": true
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success" 