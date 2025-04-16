#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Allow specifying the full image name including tag
IMAGE_NAME=${1:-"weblogproxy:latest"}

CONTAINER_NAME="weblogproxy-test"
TEST_PORT=8080

# Clean up on script exit
# shellcheck disable=SC2317  # Don't warn about unreachable commands in this function
function cleanup {
  echo "Cleaning up..."
  docker stop "$CONTAINER_NAME" >/dev/null 2>&1 || true
  docker rm "$CONTAINER_NAME" >/dev/null 2>&1 || true
  
  if [ -n "$CONTAINER_NAME_UID_TEST" ]; then
    docker stop "$CONTAINER_NAME_UID_TEST" >/dev/null 2>&1 || true
    docker rm "$CONTAINER_NAME_UID_TEST" >/dev/null 2>&1 || true
  fi
  
  echo "Done."
}
trap cleanup EXIT

echo "=== Testing Docker Image ==="
echo "Using image: $IMAGE_NAME"

# Check if image exists
if ! docker image inspect "$IMAGE_NAME" &>/dev/null; then
  echo "ERROR: Image $IMAGE_NAME does not exist. Run docker-build.sh first or specify an existing image."
  exit 1
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
      echo "Testing UID/GID functionality..."
      
      # Set test UID/GID - using values unlikely to conflict with system users
      TEST_UID=9999
      TEST_GID=9999
      CONTAINER_NAME_UID_TEST="${CONTAINER_NAME}-uid-test"
      
      echo "Running container with custom UID:$TEST_UID GID:$TEST_GID..."
      docker run -d --name $CONTAINER_NAME_UID_TEST -e PUID=$TEST_UID -e PGID=$TEST_GID "$IMAGE_NAME"
      sleep 3
      
      echo "Checking if container is running with proper UID/GID..."
      UID_OUTPUT=$(docker exec $CONTAINER_NAME_UID_TEST id -u weblogproxy)
      GID_OUTPUT=$(docker exec $CONTAINER_NAME_UID_TEST id -g weblogproxy)
      
      echo "Container user UID: $UID_OUTPUT (expected: $TEST_UID)"
      echo "Container user GID: $GID_OUTPUT (expected: $TEST_GID)"
      
      if [ "$UID_OUTPUT" = "$TEST_UID" ] && [ "$GID_OUTPUT" = "$TEST_GID" ]; then
        echo "SUCCESS: UID/GID correctly set to $TEST_UID:$TEST_GID"
        docker stop $CONTAINER_NAME_UID_TEST
        docker rm $CONTAINER_NAME_UID_TEST
        CONTAINER_NAME_UID_TEST=""
        
        echo
        echo "=== Docker test completed successfully ==="
        exit 0
      else
        echo "ERROR: UID/GID test failed"
        docker logs $CONTAINER_NAME_UID_TEST
        exit 1
      fi
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
