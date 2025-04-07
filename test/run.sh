#!/bin/bash

# --- run.sh Principles ---
# This script serves ONLY as a test runner.
# Responsibilities:
#   - Define common variables and helper functions (e.g., logging).
#   - Discover and filter .hurl test files.
#   - Execute Hurl tests sequentially. Hurl files are responsible for making ALL HTTP requests, including loops/repeats.
#   - Execute corresponding .sh validation scripts (using `source`) if they exist. These scripts ONLY validate results (e.g., check files), they DO NOT make HTTP requests.
#   - Perform generic setup (e.g., build, cleanup) and teardown.
# NO test-specific logic or HTTP requests should reside in this file or in the .sh validation scripts.
# --------------------------

# Exit immediately if a command exits with a non-zero status.
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# --- Configuration ---
NC='\033[0m' # No Color
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'

# --- Log Functions ---
log_info() {
  echo -e "${BLUE}[INFO]" "$@""${NC}"
}

log_pass() {
  echo -e "${GREEN}[PASS]" "$@""${NC}"
}

log_fail() {
  echo -e "${RED}[FAIL]" "$@""${NC}"
}

# --- Script Variables ---
APP_PATH="../weblogproxy"
CONFIG_PATH="../config/test.yaml"
TEST_DIR="api"
HURL_VAR_FILE="${TEST_DIR}/e2e_env.vars"
LOG_DIR="/tmp"
APP_LOG="${LOG_DIR}/weblogproxy.log"
LOG_FILE_PLAIN="${LOG_DIR}/weblogproxy-test-plain.log"
LOG_FILE_ROTATION="${LOG_DIR}/weblogproxy-test-rotation.log"
PID_FILE="${LOG_DIR}/weblogproxy-test.pid"

# --- Argument Parsing and Test File Discovery ---
FILTER_PATTERN=""
declare -a HURL_FILES # Explicitly declare array

TARGET_PATTERN=""
if [ "$#" -eq 1 ]; then
  FILTER_PATTERN="$1"
  log_info "Filtering test files containing: ${FILTER_PATTERN}"
  # Simple globbing pattern for ls
  TARGET_PATTERN="${TEST_DIR}/*${FILTER_PATTERN}*.hurl"
elif [ "$#" -eq 0 ]; then
  log_info "Discovering all Hurl test files in ${TEST_DIR}..."
  TARGET_PATTERN="${TEST_DIR}/*.hurl"
else # Handles > 1 argument
  log_fail "Usage: $0 [optional_filter_pattern]"
  exit 1
fi

# Use simple for loop with globbing (less safe for weird filenames, but avoids parser issues)
# Sort later if needed, but execution order might be less predictable
shopt -s nullglob # Ensure loop doesn't run if no files match
TEMP_HURL_FILES=(${TARGET_PATTERN})
shopt -u nullglob # Turn off nullglob

# Basic sorting alphabetically
HURL_FILES=($(printf "%s\\n" "${TEMP_HURL_FILES[@]}" | sort))

# Check if any files were found
if [ ${#HURL_FILES[@]} -eq 0 ]; then
  if [ -n "${FILTER_PATTERN}" ]; then
    log_fail "No Hurl test files found in '${TEST_DIR}' matching filter '${FILTER_PATTERN}'."
  else
    log_fail "No Hurl test files found in '${TEST_DIR}'."
  fi
  exit 1
fi

log_info "Found ${#HURL_FILES[@]} test file(s) to run:"
printf "  %s\\n" "${HURL_FILES[@]}"

# --- Cleanup Function Definition ---
# Define cleanup logic early
cleanup() {
  log_info "Cleanup triggered. Stopping application..."

  if [ -f "${PID_FILE}" ]; then
    # Send SIGTERM first for graceful shutdown
    kill "$(cat ${PID_FILE})" 2>/dev/null || true
    # Wait a bit
    sleep 0.5 # Shorter wait
    # Force kill if still running
    if kill -0 "$(cat ${PID_FILE})" 2>/dev/null; then
      log_info "Force killing process..."
      kill -9 "$(cat ${PID_FILE})" 2>/dev/null || true
    fi
    rm -f "${PID_FILE}"
  else
    # Fallback if PID file is missing
    pkill -f "${APP_PATH}" || true
  fi
}

# Set trap after function definition
trap cleanup EXIT SIGINT SIGTERM

# --- Main Script Execution Starts Here ---

log_info "Starting E2E tests..."

# 1. Build (Optional - uncomment if needed)
log_info "Building application..."
(cd .. && go build -o "${APP_PATH#../}" ./cmd/weblogproxy)

# 2. Cleanup old logs & PID (Initial cleanup before start)
log_info "Cleaning up old logs and PID file..."
rm -f "${LOG_FILE_PLAIN}"* "${LOG_FILE_ROTATION}"* "${PID_FILE}"

# 3. Start application in background
log_info "Starting weblogproxy in background..."
"${APP_PATH}" -config "${CONFIG_PATH}" >"${APP_LOG}" 2>&1 &
echo $! >"${PID_FILE}"

# Wait for server to start
log_info "Waiting for server to be ready..."
MAX_WAIT=30 # seconds
COUNT=0
HEALTH_URL="http://localhost:8081/__wlp_test__/health" # Adjust if port/path changes
while ! curl --output /dev/null --silent --fail ${HEALTH_URL}; do
  sleep 1
  COUNT=$((COUNT + 1))
  if [ ${COUNT} -ge ${MAX_WAIT} ]; then
    log_fail "Server did not start within ${MAX_WAIT} seconds."
    # Cleanup will be triggered by EXIT trap
    exit 1
  fi
done
log_info "Server is ready."

# 4. Run Hurl tests sequentially
log_info "Running Hurl tests..."

# Common Hurl options
HURL_COMMON_OPTS=(
  # "--test"
  # "--verbose"
  # "--include"
  "--error-format" "long"
  "--variables-file" "${HURL_VAR_FILE}"
  "--insecure"
  "--retry" "3"
  "--retry-interval" "500"
  "--connect-timeout" "5"
  "--no-color" # Remove or set based on preference
)

# --- Hurl Execution Loop ---
OVERALL_EXIT_CODE=0
for hurl_file in "${HURL_FILES[@]}"; do
  log_info "Executing: hurl ${HURL_COMMON_OPTS[*]} ${hurl_file}"

  if hurl "${HURL_COMMON_OPTS[@]}" "${hurl_file}"; then
    log_pass "Test file '${hurl_file}' passed."

    # Check for corresponding bash test script
    bash_script="${hurl_file%.hurl}.sh"
    if [ -f "${bash_script}" ]; then
      log_info "Found bash test script: ${bash_script}"
      # Execute using source to inherit functions
      if source "${bash_script}"; then
        log_pass "Bash test script '${bash_script}' passed."
      else
        log_fail "Bash test script '${bash_script}' failed."
        OVERALL_EXIT_CODE=1
      fi
    fi
  else
    HURL_EXIT_CODE=$?
    log_fail "Test file '${hurl_file}' failed with code ${HURL_EXIT_CODE}."
    OVERALL_EXIT_CODE=${HURL_EXIT_CODE} # Store the first non-zero exit code
    # Continue executing other files
    log_info "Continuing with remaining tests..."
  fi
done

if [ ${OVERALL_EXIT_CODE} -ne 0 ]; then
  log_fail "One or more tests failed. Overall exit code: ${OVERALL_EXIT_CODE}."
  exit ${OVERALL_EXIT_CODE}
else
  log_pass "All tests passed successfully."
fi

exit 0
