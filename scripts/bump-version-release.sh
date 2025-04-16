#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Parse arguments
BUMP_TYPE=""
SKIP_CONFIRMATION=false

# Process command line arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -y|--yes)
            SKIP_CONFIRMATION=true
            shift
            ;;
        major|minor|patch)
            BUMP_TYPE=$1
            shift
            ;;
        *)
            echo "Unknown option: $1"
            echo "Usage: $0 [-y|--yes] [major|minor|patch]"
            echo "Options:"
            echo "  -y, --yes    Skip confirmation prompt"
            echo "Examples:"
            echo "  $0 patch            # Increases version from 1.2.3 to 1.2.4 (with confirmation)"
            echo "  $0 -y minor         # Increases version from 1.2.3 to 1.3.0 (without confirmation)"
            echo "  $0 major --yes      # Increases version from 1.2.3 to 2.0.0 (without confirmation)"
            exit 1
            ;;
    esac
done

# Check if bump type was provided
if [ -z "$BUMP_TYPE" ]; then
    echo "Error: Bump type (major, minor, or patch) is required"
    echo "Usage: $0 [-y|--yes] [major|minor|patch]"
    exit 1
fi

# Validate bump type
if [[ ! "$BUMP_TYPE" =~ ^(major|minor|patch)$ ]]; then
    echo "Error: Bump type must be 'major', 'minor', or 'patch'"
    exit 1
fi

echo "Current version: $VERSION"

# Remove -dev suffix if present
CLEAN_VERSION=$(echo "$VERSION" | sed 's/-dev$//')
IFS='.' read -r -a VERSION_PARTS <<<"$CLEAN_VERSION"
if [ ${#VERSION_PARTS[@]} -ne 3 ]; then
    echo "Error: Current version is not in semantic versioning format (MAJOR.MINOR.PATCH)"
    exit 1
fi
MAJOR=${VERSION_PARTS[0]}
MINOR=${VERSION_PARTS[1]}
PATCH=${VERSION_PARTS[2]}

# Calculate new version based on bump type
case $BUMP_TYPE in
major)
    NEW_MAJOR=$((MAJOR + 1))
    NEW_MINOR=0
    NEW_PATCH=0
    ;;
minor)
    NEW_MAJOR=$MAJOR
    NEW_MINOR=$((MINOR + 1))
    NEW_PATCH=0
    ;;
patch)
    NEW_MAJOR=$MAJOR
    NEW_MINOR=$MINOR
    NEW_PATCH=$((PATCH + 1))
    ;;
esac

NEW_VERSION="${NEW_MAJOR}.${NEW_MINOR}.${NEW_PATCH}"
echo "New version will be: $NEW_VERSION"

# Ask for confirmation if not skipped
if [ "$SKIP_CONFIRMATION" = false ]; then
    read -p "Do you want to proceed with updating the version? (y/n) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "Version update canceled."
        exit 1
    fi
fi

# Update version in files
echo "Updating version in $VERSION_FILE..."
sed -i "s/Version = \".*\"/Version = \"$NEW_VERSION\"/g" $VERSION_FILE

echo "Updating version in README.md..."
sed -i "s/\*\*Current version: .*\*\*/\*\*Current version: $NEW_VERSION\*\*/g" README.md

# Prepare CHANGELOG entry
TODAY=$(date +%Y-%m-%d)
CHANGELOG_FILE="CHANGELOG.md"

echo "Adding new version entry to $CHANGELOG_FILE..."
# Create backup of changelog
cp $CHANGELOG_FILE "${CHANGELOG_FILE}.bak"

# Prepare the new version header
NEW_VERSION_HEADER="## [$NEW_VERSION] - $TODAY"

# Check if ## [Unreleased] section exists
if ! grep -q "^## \[Unreleased\]" "$CHANGELOG_FILE"; then
    echo "Error: '## [Unreleased]' section not found in $CHANGELOG_FILE."
    echo "Cannot automatically update changelog header. Please check the file format."
    exit 1
fi

# Replace the '## [Unreleased]' line with the new version header using sed
echo "Updating header in $CHANGELOG_FILE..."
sed -i "s|^## \[Unreleased\].*|$NEW_VERSION_HEADER|" "$CHANGELOG_FILE"

# Check if sed command was successful
if [ $? -ne 0 ]; then
    echo "Error: Failed to update header in $CHANGELOG_FILE with sed."
    # Attempt to restore backup if it exists (assuming backup was made earlier)
    if [ -f "${CHANGELOG_FILE}.bak" ]; then
        echo "Restoring from backup..."
        mv "${CHANGELOG_FILE}.bak" "$CHANGELOG_FILE"
    fi
    exit 1
fi

# Remove the backup file created earlier if sed was successful
rm -f "${CHANGELOG_FILE}.bak"

echo
echo "Version has been updated to $NEW_VERSION"
echo "Please review and edit the $CHANGELOG_FILE with appropriate changes for this release."
echo
echo "After completing the changelog, commit the changes with:"
echo "git add $VERSION_FILE README.md $CHANGELOG_FILE"
echo "git commit -m \"Bump version to $NEW_VERSION\""
echo
echo "Then run the publish script to create a tag and publish:"
echo "./scripts/publish.sh $NEW_VERSION"
