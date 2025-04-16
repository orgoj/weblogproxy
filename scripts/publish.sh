#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Kontrola verze
if [ $# -ne 1 ]; then
    echo "Použití: $0 <verze>"
    echo "Příklad: $0 1.0.0"
    exit 1
fi

PUBLISH_VERSION=$1
TAG_NAME="v$PUBLISH_VERSION"

# Kontrola GitHub údajů
if [ -z "$GITHUB_USERNAME" ] || [ -z "$REPO_NAME" ] || [ -z "$IMAGE_NAME" ]; then
    echo "CHYBA: Nelze detekovat GitHub uživatelské jméno a repozitář."
    echo "Ujistěte se, že jste v git repozitáři a remote origin směřuje na GitHub."
    exit 1
fi

# Kontrola zda existuje tag
if git tag | grep -q "^$TAG_NAME$"; then
    echo "CHYBA: Tag $TAG_NAME již existuje!"
    exit 1
fi

# Kontrola zda verze v CHANGELOG.md odpovídá
if ! grep -q "## \[$PUBLISH_VERSION\]" CHANGELOG.md; then
    echo "CHYBA: V CHANGELOG.md chybí záznam pro verzi $PUBLISH_VERSION!"
    echo "Přidejte sekci ## [$PUBLISH_VERSION] - $(date +%Y-%m-%d) do CHANGELOG.md před pokračováním."
    exit 1
fi

# Aktualizace verze v README.md
echo "Aktualizuji README.md s novou verzí..."
sed -i "s/\*\*Current version: .*\*\*/\*\*Current version: $PUBLISH_VERSION\*\*/g" README.md

# Aktualizace verze v souboru version.go
echo "Aktualizuji version.go s novou verzí..."
sed -i "s/Version = \".*\"/Version = \"$PUBLISH_VERSION\"/g" $VERSION_FILE
sed -i "s/BuildDate = \".*\"/BuildDate = \"$BUILD_DATE\"/g" $VERSION_FILE
sed -i "s/CommitHash = \".*\"/CommitHash = \"$COMMIT_HASH\"/g" $VERSION_FILE

# Kontrola zda je repozitář čistý
if ! git diff-index --quiet HEAD --; then
    echo "Byly provedeny změny v README.md a version.go. Commitnu je..."
    git add README.md $VERSION_FILE
    git commit -m "Aktualizace verze na $PUBLISH_VERSION"
fi

echo "=== Publikování verze $TAG_NAME ==="

# 1. Tag repozitáře
echo "Označuji repozitář verzí $TAG_NAME..."
git tag "$TAG_NAME"

# 2. Push do GitHubu
echo "Nahrávám změny a tag do GitHubu..."
git push origin master
git push origin "$TAG_NAME"

# 3. Build Docker image
echo "Stavím Docker image $IMAGE_NAME:$TAG_NAME..."
docker build \
    --build-arg VERSION="$PUBLISH_VERSION" \
    --build-arg BUILD_DATE="$BUILD_DATE" \
    --build-arg COMMIT_HASH="$COMMIT_HASH" \
    -t "$IMAGE_NAME:latest" \
    -t "$IMAGE_NAME:$TAG_NAME" \
    .

# 4. Push do GitHub Container Registry
echo "Nahrávám Docker image do GitHub Container Registry..."
echo "Přihlaste se prosím do GitHub Container Registry (ghcr.io) pokud jste tak ještě neučinili."
docker push "$IMAGE_NAME:latest"
docker push "$IMAGE_NAME:$TAG_NAME"

echo "=== Publikování dokončeno ==="
echo "Weblogproxy verze $PUBLISH_VERSION byla úspěšně publikována na GitHub a GitHub Container Registry."
echo "Docker image je dostupný jako: $IMAGE_NAME:$TAG_NAME a $IMAGE_NAME:latest"
