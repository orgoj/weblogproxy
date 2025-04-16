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
echo ""
echo "Usage instructions:"
echo "  Run with default user (UID 1000, GID 1000):"
echo "    docker run -p 8080:8080 $IMAGE_NAME"
echo ""
echo "  Run with custom UID/GID:"
echo "    docker run -p 8080:8080 -e PUID=<uid> -e PGID=<gid> $IMAGE_NAME"
echo ""
echo "  Example - run as current user:"
echo "    docker run -p 8080:8080 -e PUID=\$(id -u) -e PGID=\$(id -g) $IMAGE_NAME"
echo ""
