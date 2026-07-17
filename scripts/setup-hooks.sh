#!/bin/bash
# Set up Git hooks for the project

HOOKS_DIR=".git/hooks"

cat > "$HOOKS_DIR/pre-commit" << 'EOF'
#!/bin/bash
# Pre-commit hook to run formatting and basic checks

echo "Running pre-commit checks..."

# Check for large files
MAX_SIZE_MB=10
for file in $(git diff --cached --name-only); do
    if [ -f "$file" ]; then
        size=$(stat -f%z "$file" 2>/dev/null || stat -c%s "$file" 2>/dev/null)
        if [ "$size" -gt $((MAX_SIZE_MB * 1024 * 1024)) ]; then
            echo "ERROR: $file is larger than ${MAX_SIZE_MB}MB"
            exit 1
        fi
    fi
done

# Check for Go formatting
GO_FILES=$(git diff --cached --name-only --diff-filter=ACM | grep '\.go$')
if [ -n "$GO_FILES" ]; then
    for file in $GO_FILES; do
        if [ -f "$file" ]; then
            gofmt -l "$file" > /dev/null 2>&1
            if [ $? -ne 0 ]; then
                echo "ERROR: $file is not gofmt-formatted. Run 'go fmt'."
                exit 1
            fi
        fi
    done
fi

echo "Pre-commit checks passed."
EOF

chmod +x "$HOOKS_DIR/pre-commit"
echo "Git hooks installed."
