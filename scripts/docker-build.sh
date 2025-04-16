#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Allow specifying the full image name including tag
IMAGE_NAME=${1:-"weblogproxy:latest"}
# Extract the base name without tag for version tagging
BASE_NAME=$(echo "$IMAGE_NAME" | cut -d ':' -f 1)

echo "Building Docker image with version $VERSION..."
docker build -t "$IMAGE_NAME" -t "$BASE_NAME:$VERSION" \
  --build-arg VERSION="$VERSION" \
  --build-arg BUILD_DATE="$BUILD_DATE" \
  --build-arg COMMIT_HASH="$COMMIT_HASH" \
  .

echo "Build successful: $IMAGE_NAME and $BASE_NAME:$VERSION with version $VERSION"
