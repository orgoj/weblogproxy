#!/bin/bash
set -e

# Load common configuration
source "$(dirname "$0")/config.sh"

# Kontrola, že nejsme v -dev verzi
if [[ "$VERSION" == *-dev ]]; then
    echo "CHYBA: Nelze publikovat -dev verzi!"
    echo "Nejprve proveďte 'mise run version-bump-release' pro přípravu release verze."
    exit 1
fi

TAG_NAME="v$VERSION"

# Kontrola GitHub údajů
if [ -z "$GITHUB_USERNAME" ] || [ -z "$REPO_NAME" ]; then
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
if ! grep -q "## \[$VERSION\]" CHANGELOG.md; then
    echo "CHYBA: V CHANGELOG.md chybí záznam pro verzi $VERSION!"
    echo "Přidejte sekci ## [$VERSION] - $(date +%Y-%m-%d) do CHANGELOG.md před pokračováním."
    exit 1
fi

# Kontrola zda je repozitář čistý
if ! git diff-index --quiet HEAD --; then
    echo "CHYBA: Máte necommitnuté změny!"
    echo "Commitněte nebo stashněte změny před publikováním."
    exit 1
fi

echo "=== Publikování verze $TAG_NAME ==="

# 1. Tag repozitáře
echo "Označuji repozitář verzí $TAG_NAME..."
git tag "$TAG_NAME"

# 2. Push do GitHubu
echo "Nahrávám tag do GitHubu..."
git push origin "$TAG_NAME"

echo "=== Publikování dokončeno ==="
echo "Weblogproxy verze $VERSION byla úspěšně označena tagem na GitHubu."
echo "GitHub Actions nyní automaticky:"
echo "1. Sestaví a otestuje kód"
echo "2. Vytvoří Docker image"
echo "3. Publikuje image do GitHub Container Registry"
echo
echo "Sledujte průběh na: https://github.com/$GITHUB_USERNAME/$REPO_NAME/actions"
