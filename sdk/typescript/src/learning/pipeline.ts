/**
 * pipeline.ts — v6.2「内化」蒸馏管道工件消费者（TypeScript 端，矩阵 #2）
 *
 * 双线定位（docs/双线豁免矩阵.md #2）：训练连接器 Go-only（HTTP 端点调用）；
 * TS 以**工件消费者**身份对等——数据集格式（ap-dataset-v1）与影子评测报告
 * 为双线契约，本文件实现：
 *   - DatasetManifest / DistillationExample / ShadowReport 类型（与 Go
 *     internal/agent/learning/pipeline/types.go 逐字段对齐）；
 *   - 数据集解析与互证（sha256 复算 / 行数 / 格式版本 / manifest_id）；
 *   - 影子报告判据复算（Ratio ≥0.85 且 RatioLower ≥0.80；Wilson 95% 下界
 *     同口径，z=1.959963984540054）。
 *
 * 与 Go 端差异（契约内声明）：TS 不发起训练/推理（工件消费 + 校验 +
 * 判据复算），训练面委托 Go 端连接器产出的工件。
 */

// ===== 数据集契约（与 Go types.go 逐字段对齐）=====

/** 数据集格式版本（与 Go FormatVersion 一致）。 */
export const FORMAT_VERSION = 'ap-dataset-v1';

/** 数据集消息（OpenAI chat 格式对齐）。 */
export interface DatasetMessage {
  role: string;
  content: string;
  tool_calls?: string;
  tool_call_id?: string;
  name?: string;
}

/** 蒸馏数据集单条样例。 */
export interface DistillationExample {
  id: string;
  domain: string;
  messages: DatasetMessage[];
  weight: number;
}

/** 数据集清单。 */
export interface DatasetManifest {
  format_version: string;
  manifest_id: string;
  domain: string;
  count: number;
  sha256: string;
  bytes: number;
  created_at: string;
  source: string;
}

/** 影子评测报告（R3 口径：点估计 + Wilson 95% 下界）。 */
export interface ShadowReport {
  manifest_id: string;
  champion_model: string;
  shadow_model: string;
  n: number;
  champion_success: number;
  shadow_success: number;
  champion_rate: number;
  shadow_rate: number;
  shadow_wilson_lo: number;
  ratio: number;
  ratio_lower: number;
  mcnemar_p: number;
  passed: boolean;
  audit_id: string;
  created_at: string;
}

// ===== 解析与互证 =====

/** 解析 JSONL 数据集（与 Go ParseDataset 同语义；坏行显式报错）。 */
export function parseDataset(jsonl: string): DistillationExample[] {
  const out: DistillationExample[] = [];
  const lines = jsonl.replace(/\n$/, '').split('\n');
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trim() === '') {
      continue;
    }
    try {
      out.push(JSON.parse(line) as DistillationExample);
    } catch (e) {
      throw new Error(`pipeline: 第 ${i + 1} 行解析失败: ${(e as Error).message}`);
    }
  }
  return out;
}

/** SHA-256 十六进制（WebCrypto 异步；与 Go hex.EncodeToString 一致的小写形式）。 */
export async function sha256Hex(data: Uint8Array): Promise<string> {
  const digest = await crypto.subtle.digest('SHA-256', data as BufferSource);
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, '0')).join('');
}

/**
 * 验证数据集（与 Go VerifyDataset 同算法）：
 * 格式版本 / sha256 复算 / 字节数 / 行数 / manifest_id 前缀。
 * 返回 null = 通过；否则返回错误描述。
 */
export async function verifyDataset(
  jsonl: string,
  manifest: DatasetManifest,
): Promise<Error | null> {
  if (manifest.format_version !== FORMAT_VERSION) {
    return new Error(`pipeline: 数据集格式版本 ${manifest.format_version} ≠ ${FORMAT_VERSION}`);
  }
  const bytes = new TextEncoder().encode(jsonl);
  const sha = await sha256Hex(bytes);
  if (sha !== manifest.sha256) {
    return new Error(`pipeline: JSONL sha256 ${sha} ≠ 清单登记 ${manifest.sha256}`);
  }
  if (bytes.length !== manifest.bytes) {
    return new Error(`pipeline: JSONL 字节数 ${bytes.length} ≠ 清单登记 ${manifest.bytes}`);
  }
  const trimmed = jsonl.replace(/\n$/, '');
  const lines = trimmed === '' ? 0 : trimmed.split('\n').length;
  if (lines !== manifest.count) {
    return new Error(`pipeline: JSONL 行数 ${lines} ≠ 清单登记 ${manifest.count}`);
  }
  if (manifest.manifest_id !== manifest.sha256.slice(0, 16)) {
    return new Error(
      `pipeline: manifest_id ${manifest.manifest_id} ≠ sha256 前 16 位 ${manifest.sha256.slice(0, 16)}`,
    );
  }
  return null;
}

// ===== 影子报告判据复算 =====

/** Wilson 95% 成功率区间下界（与 Go pipeline.wilsonInterval 同算法）。 */
export function wilsonLower(k: number, n: number): number {
  if (n <= 0) {
    return 0;
  }
  const z = 1.959963984540054;
  const p = k / n;
  const zn = (z * z) / n;
  const denom = 1 + zn;
  const center = (p + zn / 2) / denom;
  const rad = (z / denom) * Math.sqrt((p * (1 - p)) / n + (z * z) / (4 * n * n));
  return center - rad;
}

/**
 * 复算影子报告判据（命题 1：点 ≥0.85× 且 CI 下界 ≥0.80×）。
 * 与 Go ShadowReport.Passed 同语义；返回复算结论（不信任生产方字段）。
 */
export function recomputeVerdict(rep: ShadowReport): boolean {
  if (rep.champion_rate <= 0) {
    return false;
  }
  const ratio = rep.shadow_rate / rep.champion_rate;
  const lower = wilsonLower(rep.shadow_success, rep.n) / rep.champion_rate;
  return ratio >= 0.85 && lower >= 0.8;
}
