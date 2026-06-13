# test-integration.ps1: 跨平台集成测试入口（Windows）
# 用法:
#   .\scripts\test-integration.ps1                          # 跑全部 integration 测试
#   .\scripts\test-integration.ps1 -Provider openai         # 只跑 OpenAI 相关
#   .\scripts\test-integration.ps1 -Tag integration -Timeout 5m
#
# 行为与 Makefile 的 `test-integration` 一致：
#   go test -tags=integration -timeout 5m -v ./...
#
# 但额外提供：
#   - 自动检测 API Key 环境变量并报告哪些 Provider 会被跳过
#   - 彩色输出（绿/红状态）
#   - 缺任何 Key 都不报错，缺 Key 的测试会 t.Skip
[CmdletBinding()]
param(
    [Parameter()]
    [ValidateSet("all", "openai", "anthropic", "gemini", "qwen", "glm", "deepseek", "pkg")]
    [string]$Provider = "all",

    [string]$Tag = "integration",

    [string]$Timeout = "5m",

    [string]$Path = "./agentprimordia"
)

$ErrorActionPreference = "Stop"

# 颜色
function Write-Section($text) {
    Write-Host ""
    Write-Host "=== $text ===" -ForegroundColor Cyan
}

function Write-Ok($text) {
    Write-Host "  [OK] $text" -ForegroundColor Green
}

function Write-Skip($text) {
    Write-Host "  [SKIP] $text" -ForegroundColor DarkGray
}

function Write-Warn($text) {
    Write-Host "  [WARN] $text" -ForegroundColor Yellow
}

# 进入项目根
$scriptRoot = Split-Path -Parent $MyInvocation.MyCommand.Path
$projectRoot = Resolve-Path (Join-Path $scriptRoot "..")
Push-Location $projectRoot
try {
    Write-Section "AgentPrimordia 集成测试"
    Write-Host "  Provider 过滤: $Provider"
    Write-Host "  Build tag:     $Tag"
    Write-Host "  超时:          $Timeout"
    Write-Host "  路径:          $Path"

    # 报告 API Key 状态
    Write-Section "API Key 检测"
    $keyMap = @{
        "OpenAI"    = "OPENAI_API_KEY"
        "Anthropic" = "ANTHROPIC_API_KEY"
        "Gemini"    = "GEMINI_API_KEY"
        "Qwen"      = "QWEN_API_KEY"
        "GLM"       = "GLM_API_KEY"
        "DeepSeek"  = "DEEPSEEK_API_KEY"
    }
    foreach ($name in $keyMap.Keys) {
        $envName = $keyMap[$name]
        if ($env:$envName) {
            Write-Ok "$name ($envName) 已设置"
        } else {
            Write-Skip "$name ($envName) 未设置，相关测试会 t.Skip"
        }
    }

    # 构造 -run 参数
    $runPattern = switch ($Provider) {
        "openai"    { "TestIntegration_OpenAI|TestIntegration_NewAgent|TestIntegration_NewSession" }
        "anthropic" { "TestIntegration_Anthropic" }
        "gemini"    { "TestIntegration_Gemini" }
        "qwen"      { "TestIntegration_Qwen" }
        "glm"       { "TestIntegration_GLM" }
        "deepseek"  { "TestIntegration_.*DeepSeek" }
        "pkg"       { "TestIntegration_NewAgent|TestIntegration_NewSession" }
        default     { "TestIntegration_" }
    }

    Write-Section "执行测试"
    Write-Host "  go test -tags=$Tag -timeout=$Timeout -v -run '$runPattern' $Path/..."
    Write-Host ""

    Push-Location $Path
    try {
        go test -tags=$Tag -timeout=$Timeout -v -run "$runPattern" ./...
        $exitCode = $LASTEXITCODE
    }
    finally {
        Pop-Location
    }

    Write-Section "结果"
    if ($exitCode -eq 0) {
        Write-Ok "集成测试通过（exit=0）"
    } else {
        Write-Warn "集成测试失败（exit=$exitCode）"
    }
    exit $exitCode
}
finally {
    Pop-Location
}
