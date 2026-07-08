$files = @('internal/agent/dag/dag.go', 'internal/agent/dag/builder.go', 'internal/agent/dag/delegate.go')
foreach ($f in $files) {
    $content = Get-Content $f -Raw -Encoding UTF8
    # Package name
    $content = $content -replace 'package agent', 'package dag'
    # Type references - use word boundaries, avoid already-replaced patterns
    $content = $content -replace '(?<![\w.])AgentStats(?!\w)', 'core.AgentStats'
    $content = $content -replace '(?<![\w.])StreamEventComplete(?!\w)', 'core.StreamEventComplete'
    $content = $content -replace '(?<![\w.])StreamEvent(?!\w)', 'core.StreamEvent'
    $content = $content -replace '(?<![\w.])UserMessage(?!\w)', 'core.UserMessage'
    $content = $content -replace '(?<![\w.])HookContext(?!\w)', 'hooks.HookContext'
    $content = $content -replace '(?<![\w.])HookBeforeDAGNode(?!\w)', 'hooks.HookBeforeDAGNode'
    $content = $content -replace '(?<![\w.])HookAfterDAGNode(?!\w)', 'hooks.HookAfterDAGNode'
    $content = $content -replace '(?<![\w.])HookBeforePipelineStep(?!\w)', 'hooks.HookBeforePipelineStep'
    $content = $content -replace '(?<![\w.])HookAfterPipelineStep(?!\w)', 'hooks.HookAfterPipelineStep'
    # Agent type (but not AgentDelegateNode, AgentStats, etc.)
    $content = $content -replace '(?<![\w.])Agent(?![\w])', 'core.Agent'
    # Message type (but not UserMessage which is already replaced)
    $content = $content -replace '(?<![\w.])Message(?!\w)', 'core.Message'
    # Response type (but not DAGNodeResult, etc.)
    $content = $content -replace '(?<![\w.])Response(?!\w)', 'core.Response'
    # Hooks type (but not HookContext, HookPoint, etc.)
    $content = $content -replace '(?<![\w.])Hooks(?!\w)', 'hooks.Hooks'
    Set-Content $f -Value $content -Encoding UTF8 -NoNewline
    Write-Host "Processed: $f"
}
