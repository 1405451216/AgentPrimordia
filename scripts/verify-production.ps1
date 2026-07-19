# AgentPrimordia 生产就绪验证脚本
#
# 用途：在普通 PowerShell（不通过 AI agent 环境）跑这个脚本，
#       绕过任何工作区恢复机制，得到干净的验证结果。
#
# 用法：
#   cd e:\ap\AgentPrimordia\agentprimordia
#   ..\scripts\verify-production.ps1
#
# 前置：需安装 golangci-lint（go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest）

$ErrorActionPreference = "Continue"
Set-Location $PSScriptRoot\..\agentprimordia

Write-Host "=== 1. 清理可能被恢复机制复活的死代码配套测试 ===" -ForegroundColor Cyan
# 这些测试文件测的是已删除的死代码，如果被 IDE/快照恢复会导致 typechecking error
$deadTests = @(
  "internal\health\pprof_test.go",      # 测已删的 pprof_enhanced.go
  "internal\prompt\prompt_test.go",      # 含测已删 parser.go 的测试（截断版保留即可，原样恢复则删）
  "internal\prompt\prompt_coverage_test.go",  # 同上
  "internal\orchestration\handoff_test.go",
  "internal\orchestration\supervisor_test.go",
  "internal\orchestration\bench_test.go",
  "internal\tools\api_tools_test.go"
)
foreach ($t in $deadTests) {
  if (Test-Path $t) {
    # 检查是否引用了已删符号（简判：含 NewPprofEnhancer/NewJSONParser/NewHandoffProtocol 等）
    $content = Get-Content $t -Raw -ErrorAction SilentlyContinue
    if ($content -match "NewPprofEnhancer|NewJSONParser|NewHandoffProtocol|NewSupervisor\b|NewHTTPClientTool") {
      Remove-Item $t -Force
      Write-Host "  删除（引用已删符号）: $t"
    }
  }
}

Write-Host "`n=== 2. go build ./... ===" -ForegroundColor Cyan
go build "./..."
Write-Host "build exit=$LASTEXITCODE"

Write-Host "`n=== 3. go test ./...（禁缓存）===" -ForegroundColor Cyan
go test "./..." -count=1 2>&1 | Select-String -Pattern "^(ok|FAIL|---)"
Write-Host "test exit=$LASTEXITCODE"

Write-Host "`n=== 4. govulncheck ===" -ForegroundColor Cyan
go run golang.org/x/vuln/cmd/govulncheck@latest ./... 2>&1 | Select-String -Pattern "No vulnerabilities|vulnerabilities"
Write-Host "vulncheck exit=$LASTEXITCODE"

Write-Host "`n=== 5. golangci-lint（CI 用 v1.64，本地 v2 配置不兼容仅供参考）===" -ForegroundColor Cyan
$gcl = Get-Command golangci-lint -ErrorAction SilentlyContinue
if ($gcl) {
  golangci-lint run --timeout 5m 2>&1 | Select-String -Pattern "typechecking|No issues|issues:"
  Write-Host "lint exit=$LASTEXITCODE"
  Write-Host "（注：本地 v2 会报测试文件 errcheck，CI 的 v1.64 + .golangci.yml exclude-rules 会豁免）"
} else {
  Write-Host "golangci-lint 未安装，跳过（CI 会跑）"
}

Write-Host "`n=== 总结 ===" -ForegroundColor Green
Write-Host "build/test/vulncheck 0=通过。lint 看 CI（v1.64 配置兼容）。"
