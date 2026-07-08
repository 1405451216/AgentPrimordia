$files = @('internal/agent/dag/dag_test.go', 'internal/agent/dag/builder_test.go', 'internal/agent/dag/delegate_test.go')
foreach ($f in $files) {
    $content = Get-Content $f -Raw -Encoding UTF8
    # Package name
    $content = $content -replace 'package agent', 'package dag'
    # Type references
    $content = $content -replace '(?<![\w.])AgentStats(?!\w)', 'core.AgentStats'
    $content = $content -replace '(?<![\w.])StreamEventComplete(?!\w)', 'core.StreamEventComplete'
    $content = $content -replace '(?<![\w.])StreamEvent(?!\w)', 'core.StreamEvent'
    $content = $content -replace '(?<![\w.])UserMessage(?!\w)', 'core.UserMessage'
    $content = $content -replace '(?<![\w.])HookContext(?!\w)', 'hooks.HookContext'
    $content = $content -replace '(?<![\w.])HookPoint(?!\w)', 'hooks.HookPoint'
    $content = $content -replace '(?<![\w.])HookBeforeDAGNode(?!\w)', 'hooks.HookBeforeDAGNode'
    $content = $content -replace '(?<![\w.])HookAfterDAGNode(?!\w)', 'hooks.HookAfterDAGNode'
    $content = $content -replace '(?<![\w.])NewHookManager(?!\w)', 'hooks.NewHookManager'
    $content = $content -replace '(?<![\w.])SanitizeMermaidID(?!\w)', 'SanitizeMermaidID'
    $content = $content -replace '(?<![\w.])sanitizeMermaidID(?!\w)', 'SanitizeMermaidID'
    # Agent type (avoid AgentDelegateNode, AgentStats)
    $content = $content -replace '(?<![\w.])Agent(?![\w])', 'core.Agent'
    # Message type
    $content = $content -replace '(?<![\w.])Message(?!\w)', 'core.Message'
    # Response type
    $content = $content -replace '(?<![\w.])Response(?!\w)', 'core.Response'
    # Hooks type (avoid HookContext, HookPoint, etc.)
    $content = $content -replace '(?<![\w.])Hooks(?!\w)', 'hooks.Hooks'
    # Write without BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Resolve-Path $f).Path, $content, $utf8NoBom)
    Write-Host "Processed: $f"
}
