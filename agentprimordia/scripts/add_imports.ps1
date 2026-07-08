$files = @('internal/agent/dag/dag_test.go', 'internal/agent/dag/builder_test.go', 'internal/agent/dag/delegate_test.go')
foreach ($f in $files) {
    $content = Get-Content $f -Raw -Encoding UTF8
    # Add imports after the last import line
    $importBlock = @"
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"agentprimordia/internal/agent/core"
	"agentprimordia/internal/agent/hooks"
)
"@
    # Replace the first import block
    $content = $content -creplace '(?s)import \([^)]+\)', $importBlock
    # Write without BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Resolve-Path $f).Path, $content, $utf8NoBom)
    Write-Host "Processed: $f"
}
