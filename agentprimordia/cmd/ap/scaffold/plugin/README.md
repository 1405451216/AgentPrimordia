# {{.ProjectName}}

AgentPrimordia 插件模板项目（通过 `ap init --type plugin` 生成）。

## 快速开始

```bash
# 1. 修改 plugin.json 中的 author / homepage / repository 字段
$EDITOR plugin.json

# 2. 实现工具（在 plugin.go 中编辑或新建 tools.go）
go run .

# 3. 跑测试
make test

# 4. 构建 .so 插件包
make build

# 5. 本地安装到 ~/.agentprimordia/plugins/
make install   # 或者 `cp {{.ProjectName}}.so ~/.agentprimordia/plugins/`

# 6. 签名后发布（需要 cosign 私钥）
export COSIGN_KEY=$HOME/.cosign/cosign.key
make sign
make publish
```

## 目录结构

```
{{.ProjectName}}/
├── plugin.json          # 插件清单（name / version / type / entry）
├── plugin.go            # 插件入口，实现 ap.Plugin 接口
├── plugin_test.go       # 单元测试
├── Makefile             # build / test / sign / publish
├── README.md            # 本文件
├── .github/workflows/   # GitHub Actions CI / Release
└── .gitignore
```

## 插件接口

```go
type Plugin interface {
    Name() string
    Version() string
    Tools() []ap.Tool
    Init(config map[string]any) error
    Close() error
}
```

详见 `agentprimordia/pkg` 中的 Plugin 接口定义。

## 校验清单（发布前自检）

- [ ] `plugin.json` 中 `name` 全局唯一
- [ ] `plugin.json` 中 `version` 符合 SemVer
- [ ] `plugin.go` 通过 `go vet ./...`
- [ ] `make test` 全绿
- [ ] 已用 `cosign sign-blob` 签名
- [ ] README 中有使用示例

## 许可

MIT