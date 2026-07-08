$files = @('internal/agent/dag/dag_test.go', 'internal/agent/dag/builder_test.go', 'internal/agent/dag/delegate_test.go')
foreach ($f in $files) {
    $content = Get-Content $f -Raw -Encoding UTF8
    # Remove go:build ignore tag
    $content = $content -creplace '//go:build ignore\r?\n\r?\n', ''
    # Fix sanitizeMermaidID -> SanitizeMermaidID
    $content = $content -creplace 'sanitizeMermaidID', 'SanitizeMermaidID'
    # Case-sensitive type replacements (only match capital letter types)
    # Message type (capital M, not part of another word)
    $content = $content -creplace '(?<![\w.])Message(?!\w)', 'core.Message'
    # Response type (capital R)
    $content = $content -creplace '(?<![\w.])Response(?!\w)', 'core.Response'
    # StatusIdle
    $content = $content -creplace '(?<![\w.])StatusIdle(?!\w)', 'lifecycle.StatusIdle'
    # Fix imports - add core, hooks, lifecycle
    $content = $content -creplace 'package dag\r?\n\r?\nimport \(\r?\n\t"context"\r?\n\t"errors"\r?\n\t"testing"\r?\n\)', "package dag`n`nimport (`n`t`"context`"`n`t`"errors`"`n`t`"testing`"`n`n`t`"agentprimordia/internal/agent/core`"`n`t`"agentprimordia/internal/agent/hooks`"`n`t`"agentprimordia/internal/agent/lifecycle`"`n)"
    $content = $content -creplace 'package dag\r?\n\r?\nimport \(\r?\n\t"context"\r?\n\t"errors"\r?\n\t"fmt"\r?\n\t"strings"\r?\n\t"sync"\r?\n\t"testing"\r?\n\t"time"\r?\n\)', "package dag`n`nimport (`n`t`"context`"`n`t`"errors`"`n`t`"fmt`"`n`t`"strings`"`n`t`"sync`"`n`t`"testing`"`n`t`"time`"`n`n`t`"agentprimordia/internal/agent/core`"`n`t`"agentprimordia/internal/agent/hooks`"`n`t`"agentprimordia/internal/agent/lifecycle`"`n)"
    # Write without BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Resolve-Path $f).Path, $content, $utf8NoBom)
    Write-Host "Processed: $f"
}
