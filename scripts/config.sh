#!/bin/bash
# Central configuration script for all weblogproxy scripts
# Loads version information from version.go and exports it for other scripts

# Load version from version.go
VERSION_FILE="internal/version/version.go"
if [ ! -f "$VERSION_FILE" ]; then
    echo "Error: Version file not found at $VERSION_FILE"
    exit 1
fi

# Extract version information
VERSION=$(grep -m 1 'Version = ' $VERSION_FILE | sed 's/.*Version = "\(.*\)".*/\1/')
BUILD_DATE=$(date -u +"%Y-%m-%d")
COMMIT_HASH=$(git rev-parse --short HEAD 2>/dev/null || echo "unknown")

if [ -z "$VERSION" ]; then
    echo "Error: Could not extract version from $VERSION_FILE"
    exit 1
fi

# Repository information
GITHUB_URL=$(git config --get remote.origin.url 2>/dev/null || echo "")
if [ -n "$GITHUB_URL" ]; then
    GITHUB_USERNAME=$(echo "$GITHUB_URL" | sed -n 's/.*github.com[:/]\([^/]*\)\/weblogproxy.*.git/\1/p')
    if [ -n "$GITHUB_USERNAME" ]; then
        REPO_NAME="weblogproxy"
        IMAGE_NAME="ghcr.io/$GITHUB_USERNAME/$REPO_NAME"
    fi
fi

# Print configuration if script is executed directly (not sourced)
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    echo "WebLogProxy Configuration:"
    echo "------------------------"
    echo "Version:     $VERSION"
    echo "Build Date:  $BUILD_DATE"
    echo "Commit Hash: $COMMIT_HASH"
    if [ -n "$GITHUB_USERNAME" ]; then
        echo "GitHub User: $GITHUB_USERNAME"
        echo "Image Name:  $IMAGE_NAME"
    else
        echo "GitHub User: Not configured"
    fi
fi

export VERSION
export BUILD_DATE
export COMMIT_HASH
export GITHUB_USERNAME
export REPO_NAME
export IMAGE_NAME
