#!/bin/bash
set -euo pipefail

# Only run in Claude Code on the web
if [ "${CLAUDE_CODE_REMOTE:-}" != "true" ]; then
  exit 0
fi

echo "Installing Go dependencies and tools..."

# Download Go dependencies
go mod download

# Install security tools
echo "Installing security tools (gosec, govulncheck)..."
go install github.com/securego/gosec/v2/cmd/gosec@latest
go install golang.org/x/vuln/cmd/govulncheck@latest

# Set up pre-commit hook
echo "Setting up pre-commit hook for linting and formatting..."
cat > .git/hooks/pre-commit << 'PRECOMMIT_EOF'
#!/bin/bash
set -e

echo "Running pre-commit checks..."

# Get list of staged Go files
STAGED_GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$' || true)

if [ -z "$STAGED_GO_FILES" ]; then
  echo "No Go files staged, skipping checks."
  exit 0
fi

echo "Formatting Go files..."
go fmt ./...

echo "Running go vet..."
go vet ./...

echo "Running gosec security scan..."
if command -v gosec &> /dev/null; then
  gosec ./... || {
    echo "❌ gosec found security issues"
    exit 1
  }
else
  echo "⚠️  gosec not installed, skipping"
fi

# Re-add formatted files
git add $STAGED_GO_FILES

echo "✅ Pre-commit checks passed"
PRECOMMIT_EOF

chmod +x .git/hooks/pre-commit

echo "✅ Session startup complete - Go dependencies, security tools, and pre-commit hook installed"
