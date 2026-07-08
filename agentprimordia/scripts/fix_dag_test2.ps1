$files = @('internal/agent/dag/dag_test.go', 'internal/agent/dag/builder_test.go', 'internal/agent/dag/delegate_test.go')
foreach ($f in $files) {
    $content = Get-Content $f -Raw -Encoding UTF8
    # Package name
    $content = $content -creplace 'package agent', 'package dag'
    # Use case-sensitive replacement for type references only
    # Only replace when clearly a type (preceded by space, *, [, (, <, or comma; followed by ), }, ], comma, space, or newline)
    # AgentStats (compound word, safe to replace globally)
    $content = $content -creplace '(?<![\w.])AgentStats(?!\w)', 'core.AgentStats'
    # StreamEventComplete (compound word, safe)
    $content = $content -creplace '(?<![\w.])StreamEventComplete(?!\w)', 'core.StreamEventComplete'
    # StreamEvent (but not StreamEventComplete which is already replaced)
    $content = $content -creplace '(?<![\w.])StreamEvent(?!\w)', 'core.StreamEvent'
    # UserMessage (compound word, safe)
    $content = $content -creplace '(?<![\w.])UserMessage(?!\w)', 'core.UserMessage'
    # HookContext (compound word, safe)
    $content = $content -creplace '(?<![\w.])HookContext(?!\w)', 'hooks.HookContext'
    # HookPoint (compound word, safe)
    $content = $content -creplace '(?<![\w.])HookPoint(?!\w)', 'hooks.HookPoint'
    # HookBeforeDAGNode (compound word, safe)
    $content = $content -creplace '(?<![\w.])HookBeforeDAGNode(?!\w)', 'hooks.HookBeforeDAGNode'
    # HookAfterDAGNode (compound word, safe)
    $content = $content -creplace '(?<![\w.])HookAfterDAGNode(?!\w)', 'hooks.HookAfterDAGNode'
    # NewHookManager (compound word, safe)
    $content = $content -creplace '(?<![\w.])NewHookManager(?!\w)', 'hooks.NewHookManager'
    # Write without BOM
    $utf8NoBom = New-Object System.Text.UTF8Encoding $false
    [System.IO.File]::WriteAllText((Resolve-Path $f).Path, $content, $utf8NoBom)
    Write-Host "Processed: $f"
}
