#!/bin/bash
# deprecation-check: 验证每个 // Deprecated: 都有 // Removed in vX.Y.
set -e

deprecated=$(grep -r "Deprecated:" --include="*.go" . 2>/dev/null | wc -l)
removed_in=$(grep -r "Removed in v" --include="*.go" . 2>/dev/null | wc -l)

echo "Deprecated: $deprecated"
echo "Removed in: $removed_in"

if [ "$deprecated" -gt 0 ] && [ "$removed_in" -lt "$deprecated" ]; then
    echo "ERROR: $deprecated Deprecated but only $removed_in Removed in"
    echo ""
    echo "Every // Deprecated: must have // Removed in vX.Y."
    exit 1
fi
echo "OK: Deprecated annotations complete"
