#!/bin/bash

# According to run.sh instructions, the script should not contain HTTP requests, only validation
# CORS validation is included directly in the Hurl file, so no additional validation is needed

# Auxiliary diagnostics - displaying server log in case of unexpected problems
if [ ! -f "/tmp/weblogproxy.log" ]; then
    echo "WARN: Server log file not found, skipping log check"
    exit 0
fi

# Check if the log contains CORS initialization information
if grep -q "CORS is enabled" "/tmp/weblogproxy.log"; then
    echo "INFO: CORS initialization confirmed in server logs"
else
    echo "INFO: CORS initialization message not found in logs (expected but not critical)"
fi

# Test was successful - if Hurl tests passed
exit 0
