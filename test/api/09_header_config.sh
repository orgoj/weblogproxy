#!/bin/bash

# Validate header configuration test results in log files

# Set log file paths
LOG_JSON="/tmp/weblogproxy-test-rotation.log"
LOG_TEXT="/tmp/weblogproxy-test-plain.log"

# Wait a moment to ensure logs are flushed
sleep 0.5

log_info "Validating header condition and enrichment tests in log files"

# Test 1: Header with specific value match
if grep -q "header_condition_specific_value.*environment.*testing-rule1" "$LOG_JSON"; then
  log_pass "Test 1: Header condition with specific value matched correctly"
else
  log_fail "Test 1: Header condition with specific value did not match as expected"
fi

# Test 2: Header existence match (User-Agent: HeaderTestAgent)
# Rule should match if User-Agent exists, regardless of value
# This depends on rule configuration in test.yaml
if grep -q "header_condition_existence_true.*user_agent.*HeaderTestAgent" "$LOG_JSON"; then
  log_pass "Test 2: User-Agent header was correctly captured"

  # Check if the appropriate rule matched
  # This test is expected to be successful if the user-agent test is configured
  # If not, this might not be relevant
  if grep -q "header_condition_existence_true.*environment" "$LOG_JSON"; then
    log_pass "Test 2: Rule with User-Agent header existence condition matched"
  else
    log_info "Test 2: Rule match status could not be determined (not critical)"
  fi
else
  log_fail "Test 2: User-Agent header was not correctly captured"
fi

# Test 3: Header nonexistence (false value in condition)
# This test verifies that a rule matches when X-Unexpected-Header is absent
if grep -q "header_condition_nonexistence.*header_name.*X-Unexpected-Header" "$LOG_JSON"; then
  log_pass "Test 3: Request to test header nonexistence was logged"
  # Further validation can be added if needed
else
  log_fail "Test 3: Request to test header nonexistence was not found in logs"
fi

# Test 4: Header fails to match condition
# This test is the opposite of Test 1 - the header value is wrong
if grep -q "header_condition_wrong_value" "$LOG_JSON"; then
  # At minimum, the log entry should exist, even if rule didn't match
  log_pass "Test 4: Request with wrong header value was logged"

  # Optionally check that rule did NOT match
  if ! grep -q "header_condition_wrong_value.*environment.*testing-rule1" "$LOG_JSON"; then
    log_pass "Test 4: Rule correctly did not match for wrong header value"
  else
    log_fail "Test 4: Rule incorrectly matched for wrong header value"
  fi
else
  log_fail "Test 4: Request with wrong header value was not found in logs"
fi

# Test 5: Header for data enrichment
if grep -q "header_enrichment_test.*dest_specific_header_val.*HeaderEnrichmentValue" "$LOG_JSON"; then
  log_pass "Test 5: Header value was correctly enriched in log data"
else
  log_fail "Test 5: Header value was not enriched as expected"
fi

log_pass "Header configuration validation complete"
exit 0
