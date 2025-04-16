#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Zjisti aktuální verzi bez -dev
CLEAN_VERSION=$(echo "$VERSION" | sed 's/-dev$//')
DEV_VERSION="${CLEAN_VERSION}-dev"

# Nastav -dev verzi v version.go
sed -i "s/Version = \".*\"/Version = \"$DEV_VERSION\"/g" $VERSION_FILE

echo "Set version to $DEV_VERSION in $VERSION_FILE"

# Přidej nebo udrž sekci [Unreleased] v CHANGELOG.md
CHANGELOG_FILE="CHANGELOG.md"
if ! grep -q "^## \[Unreleased\]" "$CHANGELOG_FILE"; then
    # Přidej sekci na začátek po hlavičce
    awk 'NR==1{print; print "\n## [Unreleased]\n\n### Added\n- \n\n### Changed\n- \n\n### Fixed\n- \n"; next} 1' "$CHANGELOG_FILE" >"${CHANGELOG_FILE}.new"
    mv "${CHANGELOG_FILE}.new" "$CHANGELOG_FILE"
    echo "Added [Unreleased] section to $CHANGELOG_FILE"
else
    echo "[Unreleased] section already present in $CHANGELOG_FILE"
fi

echo "Development version is now set." 