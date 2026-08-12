# testdata — 共享测试夹具目录（v6.x 评估报告 §六.3 修复）

## 约定
本目录存放跨包复用的测试静态数据（JSON / YAML / golden 文件），
供 `go test` 通过相对路径 `../testdata/...` 引用（Go 测试工作目录
为包目录，故需 `../` 前缀跳转仓库根）。

## 规则
1. **只放静态数据**：不放代码、不放生成物。
2. **包内专属夹具**放各自包的 `testdata/` 子目录（Go 会自动忽略）；
   跨包共享的放本目录。
3. **Golden 文件**：命名 `*.golden.json`，配合 `-update` flag 更新。
4. **禁止**在 testdata 中放密钥/凭据（仓库会公开）。

## 结构
- `memory/` — memory 包 RAG/Episode 夹具
