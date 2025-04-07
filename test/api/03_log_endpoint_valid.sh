#!/bin/bash

# Script logic starts here

# --- Log File Validation ---
log_info "Validating rotation log file in 03_log_endpoint_valid.sh..."

# Check if file exists
if [ ! -f "${LOG_FILE_ROTATION}" ]; then
  log_fail "Rotation log file (${LOG_FILE_ROTATION}) not found!"
  exit 1
fi

# Check if file is not empty
if [ ! -s "${LOG_FILE_ROTATION}" ]; then
  log_fail "Rotation log file (${LOG_FILE_ROTATION}) is empty!"
  exit 1
fi

# Check for expected log entry
if ! grep -q "Valid log entry test from Hurl" "${LOG_FILE_ROTATION}"; then
  log_fail "Rotation log file does not contain expected entry!"
  echo "--- First 5 lines of ${LOG_FILE_ROTATION} ---"
  head -n 5 "${LOG_FILE_ROTATION}"
  echo "-----------------------------------------"
  exit 1
fi

# Check JSON format
FIRST_LINE=$(head -n 1 "${LOG_FILE_ROTATION}")
if ! echo "${FIRST_LINE}" | grep -qE '^\{.*\}$'; then
  log_fail "Rotation log file does not start with a valid JSON object! First line:"
  echo "${FIRST_LINE}"
  exit 1
fi

# Check for required fields
if ! echo "${FIRST_LINE}" | grep -q '"level":30'; then
  log_fail "Rotation log file (JSON) missing 'level:30'! First line:"
  echo "${FIRST_LINE}"
  exit 1
fi

# Check for expected message
if ! echo "${FIRST_LINE}" | grep -q '"msg":"Valid log entry test from Hurl"'; then
  log_fail "Rotation log file (JSON) missing expected message! First line:"
  echo "${FIRST_LINE}"
  exit 1
fi

if ! echo "${FIRST_LINE}" | grep -q '"client_ip"'; then
  log_fail "Rotation log file (JSON) missing enriched 'client_ip'! First line:"
  echo "${FIRST_LINE}"
  exit 1
fi

if ! echo "${FIRST_LINE}" | grep -q '"site_id":"test-site-1"'; then
  log_fail "Rotation log file (JSON) missing expected site_id! First line:"
  echo "${FIRST_LINE}"
  exit 1
fi

log_pass "Rotation log file validated by 03_log_endpoint_valid.sh."
