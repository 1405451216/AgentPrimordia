/**
 * lifecycle.ts — 工具生命周期协议对等（TS 端，矩阵 #3：签名/工具包格式/注册客户端）
 *
 * 双线定位（docs/双线豁免矩阵.md #3）：六段生命周期状态机与信任链在 Go 端
 * （internal/tools/lifecycle/）；沙箱彩排执行 Go-only（承 B4）。TS 侧对等面：
 *   - 生命周期阶段/候选/缺口报表/复用报表类型（与 Go 逐字段对齐）；
 *   - cosign 同款签名验证（ECDSA P-256 / SHA-256；ASN.1 DER → raw 转换 +
 *     非压缩公钥导入 WebCrypto）——注册客户端在本地验签后再提交；
 *   - 注册客户端契约（HTTP 面，实现注入）。
 */

// ===== 生命周期契约类型（与 Go types 对齐）=====

export const STAGES = [
  'gap_detected',
  'generated',
  'rehearsed',
  'adversarial_tested',
  'signed_registered',
  'retired',
] as const;

export type Stage = (typeof STAGES)[number];

export const GAP_KIND_MISSING_TOOL = 'missing_tool';
export const GAP_KIND_REPEATED_FAILURE = 'repeated_failure';

/** 单条缺口（与 Go lifecycle.Gap 对齐）。 */
export interface Gap {
  kind: string;
  key: string;
  count: number;
  sample_errors?: string[];
  first_seen: string;
  last_seen: string;
}

/** 缺口审计报表（确定性聚合口径）。 */
export interface GapReport {
  window: number;
  total: number;
  gaps: Gap[];
  generated: string;
}

/** 舰队级复用报表（命题 2 口径：分母 = 注册工具数）。 */
export interface FleetReuseReport {
  window_days: number;
  registered: number;
  reused: number;
  reuse_rate: number;
  reuse_wilson_lo: number;
  total_calls: number;
  failed_calls: number;
  generated: string;
}

/** 工具包格式（注册客户端提交面；artifact 为 WASM 模块字节）。 */
export interface ToolPackage {
  id: string;
  name: string;
  domain: string;
  description: string;
  artifact_sha256: string;
  signer_key_fingerprint: string;
  stage: Stage;
}

// ===== cosign 同款签名验证（ECDSA P-256 / SHA-256）=====

/**
 * DER（ASN.1 SEQ{r,s}）→ WebCrypto raw（r||s 定长 32+32 字节）。
 * Go 端 ECDSA 签名为 ASN.1 DER；WebCrypto ECDSA 只收 IEEE P1363 raw。
 */
export function derToRaw(der: Uint8Array): Uint8Array {
  // 结构：0x30 len 0x02 rlen [r] 0x02 slen [s]
  if (der[0] !== 0x30) {
    throw new Error('lifecycle: 签名不是 DER 序列');
  }
  let off = 2;
  if (der[off] !== 0x02) {
    throw new Error('lifecycle: 签名缺少 r 整数标记');
  }
  off++;
  const rLen = der[off++];
  let r = der.slice(off, off + rLen);
  off += rLen;
  if (der[off] !== 0x02) {
    throw new Error('lifecycle: 签名缺少 s 整数标记');
  }
  off++;
  const sLen = der[off++];
  let s = der.slice(off, off + sLen);
  // 去前导零 → 左侧补齐 32 字节
  const trim = (x: Uint8Array): Uint8Array<ArrayBuffer> => {
    let i = 0;
    while (i < x.length - 1 && x[i] === 0) {
      i++;
    }
    return new Uint8Array(x.slice(i)); // 拷贝 → 独立 ArrayBuffer（TS 5.7 缓冲区类型）
  };
  r = trim(r);
  s = trim(s);
  if (r.length > 32 || s.length > 32) {
    throw new Error('lifecycle: 签名分量超出 P-256 长度');
  }
  const raw = new Uint8Array(64);
  raw.set(r, 32 - r.length);
  raw.set(s, 64 - s.length);
  return raw;
}

/** hex → Uint8Array。 */
export function hexToBytes(hex: string): Uint8Array {
  const out = new Uint8Array(hex.length / 2);
  for (let i = 0; i < out.length; i++) {
    out[i] = parseInt(hex.slice(i * 2, i * 2 + 2), 16);
  }
  return out;
}

/**
 * cosign 同款验签：SHA-256 摘要 + ECDSA P-256（DER 签名 + 非压缩公钥）。
 * 返回 null = 通过；否则返回错误描述。与 Go VerifyCandidate 守卫同结论。
 */
export async function verifyCosignSignature(
  payload: string,
  signatureDerB64: string,
  pubUncompressedB64: string,
): Promise<Error | null> {
  try {
    const der = Uint8Array.from(atob(signatureDerB64), (c) => c.charCodeAt(0));
    const pub = Uint8Array.from(atob(pubUncompressedB64), (c) => c.charCodeAt(0));
    if (pub[0] !== 0x04 || pub.length !== 65) {
      return new Error('lifecycle: 公钥不是 P-256 非压缩形式');
    }
    const raw = derToRaw(der);
    const key = await crypto.subtle.importKey(
      'raw',
      pub as BufferSource, // WebCrypto raw 格式 = 0x04||X||Y（完整非压缩点）
      { name: 'ECDSA', namedCurve: 'P-256' },
      false,
      ['verify'],
    );
    const data = new TextEncoder().encode(payload);
    const ok = await crypto.subtle.verify(
      { name: 'ECDSA', hash: 'SHA-256' },
      key,
      raw as BufferSource,
      data as BufferSource,
    );
    return ok ? null : new Error('lifecycle: 签名验证失败');
  } catch (e) {
    // 统一错误语义：本地验签门的任何失败都是拒绝结论（与 Go 同），不抛出
    return new Error(`lifecycle: 签名验证失败: ${(e as Error).message}`);
  }
}

// ===== 注册客户端契约 =====

/** RegistrationClient 注册客户端契约（实现注入：HTTP/内存皆可）。 */
export interface RegistrationClient {
  /** submit 本地验签通过后提交注册（artifact 由调用方持有，仅传哈希锚定）。 */
  submit(pkg: ToolPackage): Promise<{ accepted: boolean; detail: string }>;
  /** list 拉取当前已注册工具清单。 */
  list(): Promise<ToolPackage[]>;
}

/** InMemoryRegistrationClient 契约的确定性内存实现（测试/本地开发）。 */
export class InMemoryRegistrationClient implements RegistrationClient {
  private registry = new Map<string, ToolPackage>();

  async submit(pkg: ToolPackage): Promise<{ accepted: boolean; detail: string }> {
    if (pkg.stage !== 'signed_registered') {
      return { accepted: false, detail: `阶段 ${pkg.stage} 不是注册态` };
    }
    if (this.registry.has(pkg.id)) {
      return { accepted: false, detail: `候选 ${pkg.id} 已注册` };
    }
    this.registry.set(pkg.id, { ...pkg });
    return { accepted: true, detail: `工具 ${pkg.name} 已注册` };
  }

  async list(): Promise<ToolPackage[]> {
    return [...this.registry.values()];
  }
}
