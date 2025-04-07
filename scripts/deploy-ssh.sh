#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Parse arguments
if [ $# -lt 1 ]; then
    echo "Usage: $0 <server> [additional_ssh_options]"
    echo "Example: $0 user@example.com -v"
    exit 1
fi

SERVER=$1
shift # Remove server from arguments, rest will be passed to SSH

BUILD_TIMESTAMP=$(date -u +"%Y%m%d-%H%M%S")
IMAGE_TAG="${VERSION}-${BUILD_TIMESTAMP}"
TEMP_IMAGE_NAME="weblogproxy:${IMAGE_TAG}"

echo "=== Deploying Docker image to $SERVER ==="
echo "Version: $VERSION"
echo "Build timestamp: $BUILD_TIMESTAMP"
echo "Image tag: $IMAGE_TAG"

# Tag existing test image with timestamp
echo "Tagging existing weblogproxy:test image as $TEMP_IMAGE_NAME..."
docker tag weblogproxy:test "$TEMP_IMAGE_NAME"

# Transfer the image to the remote server
echo "Transferring image to $SERVER..."
docker save "$TEMP_IMAGE_NAME" | ssh "$@" "$SERVER" "docker load"

# Tag the image as latest
echo "Tagging image as latest..."
# shellcheck disable=SC2029
ssh "$@" "$SERVER" "docker tag ${TEMP_IMAGE_NAME} weblogproxy:latest"

# Clean up the local temporary image
echo "Cleaning up local temporary image..."
docker rmi "$TEMP_IMAGE_NAME"

echo "=== Deployment completed ==="
echo "Docker image weblogproxy:$IMAGE_TAG is now available on $SERVER"
echo
echo "To run the deployed image, execute on the remote server:"
echo "docker run -d -p 8080:8080 --name weblogproxy $TEMP_IMAGE_NAME"
