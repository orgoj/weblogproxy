# ------------------------------------------------------
# Header Configuration Test
# ------------------------------------------------------

# First, get a token for a valid site/GTM but with special site_id for header tests
GET {{logger_js_url}}?site_id={{header_test_site_id}}&gtm_id={{valid_gtm_id}}
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

# Test 1: Header with specific value in rule condition
# This test relies on the TestValue header matching a rule's condition
POST {{log_url}}
Content-Type: application/json
X-Test-Header: TestValue
{
    "token": "{{token}}",
    "site_id": "{{header_test_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Header condition test - specific value",
        "test_id": "header_condition_specific_value"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 2: Header existence condition (true)
# Tests a rule that requires header to exist, but doesn't check value
POST {{log_url}}
Content-Type: application/json
User-Agent: HeaderTestAgent
{
    "token": "{{token}}",
    "site_id": "{{header_test_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Header condition test - existence true",
        "test_id": "header_condition_existence_true"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 3: Header nonexistence condition (false)
# Tests a rule that requires header to NOT exist
POST {{log_url}}
Content-Type: application/json
{
    "token": "{{token}}",
    "site_id": "{{header_test_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Header condition test - nonexistence (false)",
        "test_id": "header_condition_nonexistence",
        "header_name": "X-Unexpected-Header"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 4: Header condition fails match
# Same test as the first one but with wrong header value
POST {{log_url}}
Content-Type: application/json
X-Test-Header: WrongValue
{
    "token": "{{token}}",
    "site_id": "{{header_test_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Header condition test - failed match",
        "test_id": "header_condition_wrong_value"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success"

# Test 5: Header for data enrichment
# Tests that header value is correctly added to log data
POST {{log_url}}
Content-Type: application/json
X-Custom-Dest-Header: HeaderEnrichmentValue
{
    "token": "{{token}}",
    "site_id": "{{header_test_site_id}}",
    "gtm_id": "{{valid_gtm_id}}",
    "data": {
        "message": "Header value for enrichment test",
        "test_id": "header_enrichment_test"
    }
}
HTTP 200
[Asserts]
    status == 200
    header "X-Log-Status" == "success" 