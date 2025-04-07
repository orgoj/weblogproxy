#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

CONTAINER_NAME="weblogproxy-test"
TEST_PORT=8080
# Allow overriding the image name
IMAGE_NAME=${1:-"weblogproxy:test"}

# Clean up on script exit
# shellcheck disable=SC2317  # Don't warn about unreachable commands in this function
function cleanup {
  echo "Cleaning up..."
  docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
  echo "Done."
}
trap cleanup EXIT

# Skip build if image name is provided
if [ "$#" -eq 0 ]; then
  echo "=== Testing Docker Build & Run ==="
  echo "Building Docker image with version $VERSION..."
  docker build -t "$IMAGE_NAME" \
    --build-arg VERSION="$VERSION" \
    --build-arg BUILD_DATE="$BUILD_DATE" \
    --build-arg COMMIT_HASH="$COMMIT_HASH" \
    .
else
  echo "=== Testing Docker Run ==="
  echo "Using provided image: $IMAGE_NAME"
fi

echo "Running Docker container..."
docker run -d --name $CONTAINER_NAME -p $TEST_PORT:8080 "$IMAGE_NAME"
echo "Waiting for container to start..."
sleep 2

echo "Testing container health..."
if ! docker ps | grep -q $CONTAINER_NAME; then
  echo "ERROR: Container is not running"
  docker logs $CONTAINER_NAME
  exit 1
fi

echo "Checking container logs..."
# Get container logs
set -x
docker logs $CONTAINER_NAME | head -1
FIRST_LOG_LINE=$(docker logs $CONTAINER_NAME | head -1)
echo "Expected version log to contain: WebLogProxy version $VERSION"

if echo "$FIRST_LOG_LINE" | grep -q "WebLogProxy version $VERSION"; then
  echo "SUCCESS: Container logs contain correct version"
else
  echo "WARNING: Container logs might not contain correct version information"
  echo "This might be due to logging format differences, continuing with API tests"
fi
set +x

echo "Testing HTTP endpoints..."
HEALTH_URL="http://localhost:$TEST_PORT/health"
VERSION_URL="http://localhost:$TEST_PORT/version"
MAX_ATTEMPTS=5
ATTEMPT=0

while [ $ATTEMPT -lt $MAX_ATTEMPTS ]; do
  ATTEMPT=$((ATTEMPT + 1))
  echo "Attempt $ATTEMPT/$MAX_ATTEMPTS: Testing health endpoint..."

  set -x
  HEALTH_RESPONSE=$(curl -s $HEALTH_URL)
  echo "Health response: $HEALTH_RESPONSE"

  if echo "$HEALTH_RESPONSE" | grep -q "status.*ok"; then
    set +x
    echo "SUCCESS: Health check passed"
    echo
    echo "Testing version endpoint..."

    set -x
    VERSION_OUTPUT=$(curl -s $VERSION_URL)
    echo "Version endpoint output: $VERSION_OUTPUT"
    set +x

    # Verify version in endpoint response
    if echo "$VERSION_OUTPUT" | grep -q "\"version\":\"$VERSION\""; then
      echo "SUCCESS: Version endpoint returned correct version"
      echo
      echo "=== Docker test completed successfully ==="
      exit 0
    else
      echo "ERROR: Version endpoint returned incorrect version"
      echo "Expected version: $VERSION"
      echo "Received: $VERSION_OUTPUT"
      exit 1
    fi
  fi
  set +x

  echo "Health check failed, waiting before retry..."
  sleep 3
done

echo "ERROR: Health check failed after $MAX_ATTEMPTS attempts"
docker logs $CONTAINER_NAME
exit 1
