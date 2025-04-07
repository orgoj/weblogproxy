#!/bin/bash

# Validace formátu času v JSON logu
log_info "Validating time format in JSON log..."

# Počkáme chvíli, aby se log zapsal na disk
sleep 1

# Hledáme test_id v logu, které jsme právě poslali
if [ ! -f "${LOG_FILE_ROTATION}" ]; then
  log_fail "Rotation log file (${LOG_FILE_ROTATION}) not found!"
  exit 1
fi

# Najdeme řádek s naším testem
TEST_LINE=$(grep "time_format_test" "${LOG_FILE_ROTATION}" | tail -n 1)
if [ -z "$TEST_LINE" ]; then
  log_fail "Test log entry not found in log file!"
  exit 1
fi

log_info "Found test log entry:"
echo "$TEST_LINE"

# Zkontrolujeme, že záznam obsahuje pole "time"
if ! echo "$TEST_LINE" | grep -q '"time":'; then
  log_fail "Log entry missing 'time' field!"
  exit 1
fi

# Zkontrolujeme, že hodnota "time" je string (v uvozovkách), ne číslo
if echo "$TEST_LINE" | grep -q '"time":[0-9]'; then
  log_fail "Time field is in numeric format (e.g. 1743729151322), should be ISO 8601 string!"
  exit 1
fi

# Ověříme, že časový formát je ISO 8601 string v uvozovkách (např. "2024-04-01T12:34:56.789Z")
if ! echo "$TEST_LINE" | grep -q '"time":"[0-9]\{4\}-[0-9]\{2\}-[0-9]\{2\}T[0-9]\{2\}:[0-9]\{2\}:[0-9]\{2\}.[0-9]\{1,\}Z"'; then
  log_fail "Time field is not in ISO 8601 format (e.g. \"2024-04-01T12:34:56.789Z\")!"
  exit 1
fi

log_pass "Time format is correctly set to ISO 8601 string."
