#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

dry_run() {
  # If input is being piped (! -t 0), display it first using cat.
  if ! [ -t 0 ]; then
    cat
    echo -en "\e[38;5;208m | \e[0m"
  else
    echo -en "\e[32mDRY-RUN: \e[0m"
  fi
  echo "$*"
}

R=
if [[ "$1" == "--dry-run" ]]; then
  R="dry_run"
  shift
fi

# Parse arguments
if [ $# -lt 1 ]; then
    echo "Usage: $0 <server> [additional_ssh_options]"
    echo "Example: $0 user@example.com -v"
    exit 1
fi

SERVER=$1
shift # Remove server from arguments, rest will be passed to SSH

BUILD_TIMESTAMP=$(docker inspect --format='{{.Created}}' weblogproxy:latest | sed -E 's/([0-9]{4})-([0-9]{2})-([0-9]{2})T([0-9]{2}):([0-9]{2}):([0-9]{2}).*/\1\2\3-\4\5\6/')
IMAGE_TAG="${VERSION}-${BUILD_TIMESTAMP}"
TEMP_IMAGE_NAME="weblogproxy:${IMAGE_TAG}"

echo "=== Deploying Docker image to $SERVER ==="
echo "Version: $VERSION"
echo "Build timestamp: $BUILD_TIMESTAMP"
echo "Image tag: $IMAGE_TAG"

# Tag existing test image with timestamp
echo "Tagging existing weblogproxy:test image as $TEMP_IMAGE_NAME..."
$R docker tag weblogproxy:test "$TEMP_IMAGE_NAME"

# Transfer the image to the remote server
echo "Transferring image to $SERVER..."
$R docker save "$TEMP_IMAGE_NAME" | $R ssh "$@" "$SERVER" "docker load"

# Tag the image as latest
echo "Tagging image as latest..."
# shellcheck disable=SC2029
$R ssh "$@" "$SERVER" "docker tag ${TEMP_IMAGE_NAME} weblogproxy:latest"

# Clean up the local temporary image
echo "Cleaning up local temporary image..."
$R docker rmi "$TEMP_IMAGE_NAME"

echo "=== Deployment completed ==="
echo "Docker image weblogproxy:$IMAGE_TAG is now available on $SERVER"
echo
echo "To run the deployed image, execute on the remote server:"
echo "docker run -d -p 8080:8080 --name weblogproxy $TEMP_IMAGE_NAME"
