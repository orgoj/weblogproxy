#!/bin/bash

# Get current UID and GID
USER_ID=${PUID:-1000}
GROUP_ID=${PGID:-1000}

echo "Starting with UID: $USER_ID, GID: $GROUP_ID"

# Update the user and group IDs if needed
if [ "$USER_ID" != "1000" ] || [ "$GROUP_ID" != "1000" ]; then
    echo "Updating weblogproxy user/group with new UID: $USER_ID / GID: $GROUP_ID"
    groupmod -g $GROUP_ID weblogproxy
    usermod -u $USER_ID weblogproxy
    
    # Update ownership of necessary files
    chown -R weblogproxy:weblogproxy /app
fi

# Execute the command as weblogproxy user
# Make sure the first argument is a full path if it doesn't contain a path separator
if [ "$1" != "" ] && [[ "$1" != */* ]]; then
    exec su-exec weblogproxy /usr/local/bin/"$1" "${@:2}"
else
    exec su-exec weblogproxy "$@" 
fi 