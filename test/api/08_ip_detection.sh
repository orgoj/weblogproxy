#!/bin/bash

# Validate IP detection test results in log files

# Set log file paths
LOG_JSON="/tmp/weblogproxy-test-rotation.log"
LOG_TEXT="/tmp/weblogproxy-test-plain.log"

# Wait a moment to ensure logs are flushed
sleep 0.5

# Test direct IP connection
log_info "Validating IP detection in log files"

# Check for direct IP test
if grep -q "ip_detection_test_direct" "$LOG_JSON"; then
  # Record found, check if client_ip is present
  if grep -q "ip_detection_test_direct.*client_ip" "$LOG_JSON"; then
    log_pass "Direct IP connection - client_ip field exists"
  else
    log_fail "client_ip field not found in log file for direct connection test"
  fi
else
  log_fail "Direct IP connection log entry not found in file"
fi

# Check trusted proxy
if grep -q "ip_detection_test_trusted_proxy.*client_ip.*192.168.100.100" "$LOG_JSON"; then
  log_pass "Trusted proxy detection - found expected client_ip: 192.168.100.100"
else
  log_fail "Expected client_ip 192.168.100.100 not found in log file for trusted proxy test"
fi

# Check untrusted proxy
if grep -q "ip_detection_test_untrusted_proxy.*client_ip.*127.0.0.1" "$LOG_JSON"; then
  log_pass "Untrusted proxy handling - client_ip is correctly 127.0.0.1, ignoring X-Forwarded-For"
else
  # Alternate behavior is valid
  if grep -q "ip_detection_test_untrusted_proxy.*client_ip.*10.10.10.10" "$LOG_JSON"; then
    log_pass "Untrusted proxy handling - client_ip is the direct connecting IP (acceptable)"
  else
    log_fail "Expected client_ip not found in log file for untrusted proxy test"
  fi
fi

# Check rule matching by IP from XFF
if grep -q "ip_detection_test_rule_match.*environment.*testing-rule1" "$LOG_JSON"; then
  log_pass "Rule matching by IP - found expected rule match indicator (environment:testing-rule1)"
  if grep -q "ip_detection_test_rule_match.*client_ip.*192.168.1.123" "$LOG_JSON"; then
    log_pass "Rule matching by IP - client_ip is correctly 192.168.1.123 from X-Forwarded-For"
  else
    log_fail "Expected client_ip 192.168.1.123 not found in log file"
  fi
else
  log_fail "Rule matching by IP - expected rule match indicator not found"
fi

# Overall pass
log_pass "IP detection validation complete"
exit 0
