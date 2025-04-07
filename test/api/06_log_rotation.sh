#!/bin/bash
# Validation script for 06_log_rotation.hurl

# --- Rotation File Validation ---
log_info "Validating log rotation for ${LOG_FILE_ROTATION}..."

# Give the logger a moment to potentially finish writing and rotating
sleep 1

# Check if the *original* log file still exists
if [ ! -f "${LOG_FILE_ROTATION}" ]; then
  log_fail "Original rotation log file (${LOG_FILE_ROTATION}) not found after rotation test!"
  exit 1
fi

# Check for the existence of at least one rotated file
# Format is typically filename.YYYY-MM-DDTHH-MM-SS.fffffffff.log or similar if using lumberjack defaults
# Or filename.N if using simpler backup naming (less common with lumberjack)
# We will use a glob pattern to find potential backup files.
shopt -s nullglob                            # Prevent error if no files match
ROTATED_FILES=("${LOG_FILE_ROTATION%.log}"*) # Basic glob, adjust if needed
shopt -u nullglob

if [ ${#ROTATED_FILES[@]} -eq 0 ]; then
  log_fail "No rotated log files found matching!"
  ls -la "${LOG_DIR}/weblogproxy"* # List directory contents for debugging
  exit 1
fi

log_pass "Found ${#ROTATED_FILES[@]} rotated log file(s). Rotation appears to have occurred."
printf "  Found: %s\n" "${ROTATED_FILES[@]}"

if [ ${#ROTATED_FILES[@]} -le 1 ]; then
  log_fail "Only one rotated log file found matching!"
  ls -la "${LOG_DIR}/weblogproxy"* # List directory contents for debugging
  #cat "${LOG_FILE_ROTATION}" # List contents of original log file
  exit 1
fi

# Optional: Check if the main log file size is small (indicating rotation occurred recently)
# CURRENT_SIZE=$(stat -c%s "${LOG_FILE_ROTATION}")
# MAX_EXPECTED_SIZE=10000 # Check if it's less than ~10KB (adjust as needed)
# if [ "${CURRENT_SIZE}" -gt "${MAX_EXPECTED_SIZE}" ]; then
#   log_warn "Original log file size (${CURRENT_SIZE} bytes) seems large after rotation test, maybe rotation didn't trigger recently?"
# fi

log_pass "Log rotation validation complete."
