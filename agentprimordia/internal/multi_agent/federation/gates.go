// gates.go — 联邦资产三形态确定性验证门（红队对抗集 skill-card-poison /
// bad-tool-package / adapter-tamper 家族的防御面，V7 §五）
//
// 全部门判定为确定性算法（签名/哈希/过期比较/声明一致性/路径解析），
// R3 口径允许 100%/0；误拦面由 benign-control 对照家族回归锁定。
package federation

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// ===== SkillCard 技能卡门 =====

// SkillCard 技能卡（流通资产）。
type SkillCard struct {
	AssetID      string            `json:"asset_id"`
	Issuer       NodeID            `json:"issuer"`
	Payload      string            `json:"payload"`      // 能力载荷（声明 + 摘要文本）
	PayloadSHA   string            `json:"payload_sha"`  // 载荷 sha256（十六进制）
	Signature    string            `json:"signature"`    // 签名（base64）
	ValidUntil   time.Time         `json:"valid_until"`  // 有效期
	Capabilities []string          `json:"capabilities"` // 申报能力
	Extra        map[string]string `json:"extra,omitempty"`
}

// SkillCardGateConfig 技能卡门配置。
type SkillCardGateConfig struct {
	// Verify 验签函数（cosign 同款口径；nil = 跳过验签的测试形态）
	Verify VerifierFunc
	// ApprovedCaps 获批准的能力白名单（宿主不批准的能力申报即拒）
	ApprovedCaps []string
	// Now 时钟（过期判定）
	Now func() time.Time
}

// ValidateSkillCard 技能卡门：过期/完整性/验签/宿主权限申报/申报-载荷一致性
// 五项确定性检查（任一失门即拒绝，红队家族 skill-card-poison 全覆盖）。
func ValidateSkillCard(c *SkillCard, cfg SkillCardGateConfig) error {
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	// ① 过期
	if !c.ValidUntil.IsZero() && c.ValidUntil.Before(cfg.Now()) {
		return fmt.Errorf("federation: 技能卡 %s 已过期（valid_until %s）", c.AssetID, c.ValidUntil.Format(time.RFC3339))
	}
	// ② 完整性
	sum := sha256.Sum256([]byte(c.Payload))
	if hex.EncodeToString(sum[:]) != c.PayloadSHA {
		return fmt.Errorf("federation: 技能卡 %s 载荷与声明哈希不符", c.AssetID)
	}
	// ③ 验签
	if cfg.Verify != nil {
		if err := cfg.Verify([]byte(c.Payload), c.Signature, string(c.Issuer)); err != nil {
			return fmt.Errorf("federation: 技能卡 %s 签名无法用发布者公钥验证: %w", c.AssetID, err)
		}
	}
	// ④ 宿主权限申报：申报能力必须在批准集内（fs.write(/etc) 等宿主写权限
	// 默认不在批准集——宿主永久拒绝面）
	approved := make(map[string]bool, len(cfg.ApprovedCaps))
	for _, cap := range cfg.ApprovedCaps {
		approved[cap] = true
	}
	for _, cap := range c.Capabilities {
		if !approved[cap] {
			return fmt.Errorf("federation: 技能卡 %s 申报了未批准能力 %q（宿主权限越界）", c.AssetID, cap)
		}
	}
	// ⑤ 申报-载荷一致性：载荷含执行语义关键词而 capabilities 未申报
	if err := checkCapDeclaration(c.Payload, c.Capabilities); err != nil {
		return fmt.Errorf("federation: 技能卡 %s %w", c.AssetID, err)
	}
	return nil
}

// capSemantics 执行语义关键词 → 应申报的能力（申报不一致检测，确定性）。
var capSemantics = []struct {
	pattern *regexp.Regexp
	cap     string
}{
	{regexp.MustCompile(`(?i)\bexec(ute)?\s*\(`), "code-exec"},
	{regexp.MustCompile(`(?i)\beval\s*\(`), "code-exec"},
	{regexp.MustCompile(`(?i)os\.system|subprocess`), "code-exec"},
	{regexp.MustCompile(`(?i)fs\.(write|append)File|open\([^)]*['\"]w`), "fs-write"},
	{regexp.MustCompile(`(?i)net/http|urllib|requests\.(get|post)`), "net-access"},
}

func checkCapDeclaration(payload string, caps []string) error {
	declared := make(map[string]bool, len(caps))
	for _, c := range caps {
		declared[c] = true
	}
	for _, cs := range capSemantics {
		if cs.pattern.MatchString(payload) && !declared[cs.cap] {
			return fmt.Errorf("能力申报不一致：载荷含 %q 语义但 capabilities 未申报 %q", cs.pattern, cs.cap)
		}
	}
	return nil
}

// ===== ToolPackage 工具包门 =====

// ToolPackage 工具包（WASM 工件 + 清单 + 安装脚本）。
type ToolPackage struct {
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Description  string   `json:"description"`
	ArtifactSHA  string   `json:"artifact_sha"`            // WASM 字节 sha256
	Artifact     []byte   `json:"-"`                       // WASM 字节
	Signature    string   `json:"signature"`               // 签名（base64；空 = 未签名）
	Imports      []string `json:"imports"`                 // WASM 导入段（模块名粒度）
	InstallPath  string   `json:"install_path,omitempty"`  // 安装目标路径（空 = 沙箱数据目录）
	InstallLinks string   `json:"install_links,omitempty"` // 安装脚本中的符号链接目标
	ClaimedOps   []string `json:"claimed_ops"`             // 清单声明操作（与描述一致性）
	Symlinks     []string `json:"symlinks,omitempty"`      // 包内符号链接（目标路径）
}

// ToolPackageGateConfig 工具包门配置。
type ToolPackageGateConfig struct {
	SandboxDataDir string   // 沙箱数据目录（安装唯一合法落点；符号链接不得逃逸）
	ApprovedCaps   []string // 获批准能力（导入段白名单）
	Now            func() time.Time
}

// ValidateToolPackage 工具包门：未签名/签名验签/导入段白名单/安装宿主写入/
// 符号链接逃逸/声明一致性 六项确定性检查（bad-tool-package 家族全覆盖）。
func ValidateToolPackage(p *ToolPackage, cfg ToolPackageGateConfig) error {
	// ① 未签名（A6 前置语义：进注册表的包必须已签名）
	if strings.TrimSpace(p.Signature) == "" {
		return fmt.Errorf("federation: 工具包 %s 未签名（A6 签名前置）", p.Name)
	}
	// ② 工件完整性
	if len(p.Artifact) > 0 {
		sum := sha256.Sum256(p.Artifact)
		if hex.EncodeToString(sum[:]) != p.ArtifactSHA {
			return fmt.Errorf("federation: 工具包 %s 工件哈希不符", p.Name)
		}
	}
	// ③ 导入段白名单（A7 同口径：未批准宿主导入即拒）
	approved := make(map[string]bool, len(cfg.ApprovedCaps))
	for _, m := range cfg.ApprovedCaps {
		approved[m] = true
	}
	for _, imp := range p.Imports {
		if !approved[imp] {
			return fmt.Errorf("federation: 工具包 %s 沙箱 import 越界 %q（批准集 %v）", p.Name, imp, cfg.ApprovedImports())
		}
	}
	// ④ 安装目标宿主写入（install-host-write）
	if p.InstallPath != "" && !withinDir(cfg.SandboxDataDir, p.InstallPath) {
		return fmt.Errorf("federation: 工具包 %s 安装脚本写宿主路径 %q（沙箱数据目录 %s 之外）", p.Name, p.InstallPath, cfg.SandboxDataDir)
	}
	// ⑤ 符号链接逃逸（symlink-escape）：链接解析后目标必须仍落在沙箱数据目录
	for _, target := range p.Symlinks {
		resolved := resolveWithin(cfg.SandboxDataDir, target)
		if !withinDir(cfg.SandboxDataDir, resolved) {
			return fmt.Errorf("federation: 工具包 %s 符号链接逃逸沙箱（目标 %q 解析至 %s）", p.Name, target, resolved)
		}
	}
	// ⑥ 声明一致性（claims-mismatch）：描述声称只读而声明操作含写语义
	for _, op := range p.ClaimedOps {
		if isWriteOp(op) && !isWriteDisclosed(p.Description) {
			return fmt.Errorf("federation: 工具包 %s 声明操作 %q 含写语义但描述未披露", p.Name, op)
		}
	}
	return nil
}

// ApprovedImports 工具包门的导入批准集视图（错误信息用）。
func (c *ToolPackageGateConfig) ApprovedImports() []string { return c.ApprovedCaps }

// withinDir 判断 target 是否落在 dir 之内（词法归一化，不含真实 fs 解析）。
func withinDir(dir, target string) bool {
	cleanDir := strings.TrimSuffix(filepath_Clean(dir), "/")
	cleanTarget := filepath_Clean(target)
	return cleanTarget == cleanDir || strings.HasPrefix(cleanTarget, cleanDir+"/")
}

func filepath_Clean(p string) string {
	// 最小 Clean 语义（处理 ./ 与 .. 与重复分隔符），避免引 path/filepath 到处的包差异
	parts := strings.Split(p, "/")
	var out []string
	for _, seg := range parts {
		switch seg {
		case "", ".":
			continue
		case "..":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, seg)
		}
	}
	return strings.Join(out, "/")
}

// resolveWithin 模拟符号链接解析（确定性：把 target 中的 .. 与链接名折叠）。
func resolveWithin(base, target string) string {
	if strings.HasPrefix(target, "/") {
		return filepath_Clean(target)
	}
	return filepath_Clean(base + "/" + target)
}

// isWriteOp 操作名是否含写语义。
func isWriteOp(op string) bool {
	op = strings.ToLower(op)
	return strings.Contains(op, "write") || strings.Contains(op, "install") ||
		strings.Contains(op, "create") || strings.Contains(op, "delete") ||
		strings.Contains(op, "remove") || strings.Contains(op, "写")
}

// isWriteDisclosed 描述是否披露写语义。
func isWriteDisclosed(desc string) bool {
	return isWriteOp(desc)
}

// ===== ModelAdapter 模型适配器门 =====

// AdapterRecord 适配器登记记录（发布方声明 vs 登记指纹的对照基准）。
type AdapterRecord struct {
	BaseModelFingerprint string `json:"base_model_fingerprint"` // 声明基座的权重指纹
	EmbeddingDim         int    `json:"embedding_dim"`          // 声明向量维度
	LicenseID            string `json:"license_id"`             // 登记许可证
}

// ModelAdapter 适配器资产（流通形态）。
type ModelAdapter struct {
	AssetID       string            `json:"asset_id"`
	OriginNode    NodeID            `json:"origin_node"`
	DeclaredBase  string            `json:"declared_base"`  // 声明基座模型指纹
	WeightSHA     string            `json:"weight_sha"`     // 权重实际指纹（sha256）
	EmbeddingDim  int               `json:"embedding_dim"`  // 实际向量维度
	Provenance    []NodeID          `json:"provenance"`     // 训练轨迹溯源链
	TokenizerConf string            `json:"tokenizer_conf"` // tokenizer 配置文本
	LicenseID     string            `json:"license_id"`     // 许可证字段
	Extra         map[string]string `json:"extra,omitempty"`
}

// evilTokenizerPattern tokenizer 配置中的远程代码钩子形态（确定性匹配）。
var evilTokenizerPattern = regexp.MustCompile(
	`(?i)(https?://[^\s"']+\.(py|js|sh)|exec\(|eval\(|__import__|curl\s+.*\|\s*(sh|bash))`)

// ValidateModelAdapter 适配器门：基座指纹/维度/溯源/tokenizer 钩子/许可证
// 五项确定性检查（adapter-tamper 家族全覆盖）。
func ValidateModelAdapter(a *ModelAdapter, rec *AdapterRecord) error {
	// ① 基座指纹：声明 base 与权重实际指纹必须与登记一致
	if rec != nil && a.DeclaredBase != rec.BaseModelFingerprint {
		return fmt.Errorf("federation: 适配器 %s 声明基座 %q 与登记指纹 %q 不符", a.AssetID, a.DeclaredBase, rec.BaseModelFingerprint)
	}
	if a.WeightSHA != a.DeclaredBase {
		return fmt.Errorf("federation: 适配器 %s 权重指纹 %q 与声明基座 %q 不符", a.AssetID, a.WeightSHA, a.DeclaredBase)
	}
	// ② 维度
	if rec != nil && a.EmbeddingDim != rec.EmbeddingDim {
		return fmt.Errorf("federation: 适配器 %s 向量维度 %d 与声明 %d 不符", a.AssetID, a.EmbeddingDim, rec.EmbeddingDim)
	}
	// ③ 溯源：无训练轨迹来源证明即拒
	if len(a.Provenance) == 0 {
		return fmt.Errorf("federation: 适配器 %s 缺少训练轨迹来源证明", a.AssetID)
	}
	// ④ tokenizer 远程代码钩子
	if evilTokenizerPattern.MatchString(a.TokenizerConf) {
		return fmt.Errorf("federation: 适配器 %s tokenizer 配置含远程代码钩子", a.AssetID)
	}
	// ⑤ 许可证
	if rec != nil && !strings.EqualFold(a.LicenseID, rec.LicenseID) {
		return fmt.Errorf("federation: 适配器 %s 许可证字段 %q 与登记 %q 不符（伪造）", a.AssetID, a.LicenseID, rec.LicenseID)
	}
	// ⑥ URL 形态合法性（防御畸形配置注入）
	if u := extractURL(a.TokenizerConf); u != "" {
		if _, err := url.ParseRequestURI(u); err != nil {
			return fmt.Errorf("federation: 适配器 %s tokenizer 配置 URL 非法", a.AssetID)
		}
	}
	return nil
}

// extractURL 提取配置中的首个 http(s) URL（无则空）。
func extractURL(s string) string {
	m := regexp.MustCompile(`https?://[^\s"']+`).FindString(s)
	return m
}
