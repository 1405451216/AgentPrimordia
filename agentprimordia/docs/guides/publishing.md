# 发布指南（插件 / 技能 / 模板）

> v4.4-2 插件 SDK 成熟：`ap plugin create` 脚手架（模板/测试/发布指引）+ 统一发布流程。
> 覆盖三种生态产物：**插件**（Go .so）、**技能**（Skill manifest）、**模板**（远程模板清单）。

## 一、插件发布（`ap plugin create` 脚手架）

```bash
# 1. 脚手架生成（含 plugin.go / plugin.json / Makefile / CI / 测试）
ap plugin create ap-plugin-weather
cd ap-plugin-weather

# 2. 本地验证
make test        # 单元测试
make build       # 构建 .so

# 3. 签名（ECDSA P-256；cosign 私钥）
export COSIGN_KEY=$HOME/.cosign/cosign.key
make sign

# 4. 发布：托管 Manifest + artifact 到 HTTPS 端点
make publish     # 上传 artifact → 生成 Manifest（name/version/artifact_url/signature/public_key）

# 5. 消费者安装（强制验签）
ap plugin install https://registry.example.com/plugins/weather/manifest.json
```

发布方维护 `manifest.json`（见 `internal/marketplace` 的 `Manifest` 结构）：
`name / version / description / import_path / artifact_url / signature / public_key / published_at`。

## 二、技能发布（Skill manifest，v4.4-1）

技能以自包含清单发布，订阅方**验签 → 规范校验 → 安全扫描 → 入库**：

```json
{
  "skill": "{\"id\":\"skill-...\",\"name\":\"数据修复\",\"description\":\"...\",\"steps\":[...]}",
  "version": "1.0.0",
  "signature": "base64(ECDSA-P256-SHA256(skill JSON))",
  "public_key": "-----BEGIN PUBLIC KEY-----...",
  "published_at": "2026-08-09T00:00:00Z"
}
```

发布/订阅（Go）：

```go
// 发布方
manifest, _ := skills.SignSkillManifest(skill, privateKeyPEM)
data, _ := json.Marshal(manifest) // 托管到 HTTPS

// 订阅方
var m skills.SkillManifest
json.Unmarshal(remoteBytes, &m)
installed, err := skills.InstallSkillFromManifest(&m, store) // 验签+校验+入库
```

安全属性：
- 篡改技能 JSON → 验签失败拒绝
- 替换公钥 → 验签失败拒绝
- 危险工具（shell_exec 等）→ `SecurityScan` 拦截不入库

## 三、模板发布（远程模板清单，v4.4-3）

```json
{
  "id": "my-template",
  "name": "模板名",
  "description": "...",
  "version": "1.0.0",
  "author": "...",
  "category": "coding",
  "system_prompt": "You are...",
  "signature": "base64(ECDSA-P256-SHA256(files JSON))",
  "public_key": "-----BEGIN PUBLIC KEY-----...",
  "files": { "agent.json": "{...}" }
}
```

```bash
ap marketplace install https://registry.example.com/templates/my-template/manifest.json
```

- 带签名清单强制验签；未签名清单仅限可信源（安装时警告）。

## 四、密钥管理建议

- 私钥只存发布机/CI Secrets，永不上库
- 每个产物独立密钥对（泄露可单独吊销）
- 公钥随清单分发，消费者首次安装后固定信任（TOFU）
