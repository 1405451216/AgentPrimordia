/**
 * protocol.ts — 联邦协议 TS 端（矩阵 #5：组织客户端 + 资产消费者）
 *
 * 双线定位：黑板/租约/联邦协议对等（Go internal/multi_agent/federation/）；
 * etcd/gRPC 节点总线 Go-only（TS 经 A2A/HTTP 面接入）。本文件实现
 * **资产消费者**侧的接收门（与 Go TrustLayer.ReceiveAsset 逐门对齐）：
 *   门 1 完整性（payload sha256）→ 门 2 钉扎验签（复用 tools/lifecycle 的
 *   cosign 同款 WebCrypto 验签）→ 门 3 溯源回环 + 重签刷分指纹。
 */

// ===== 联邦契约类型（与 Go types.go 逐字段对齐）=====

export const ASSET_KINDS = ['skill-card', 'tool-package', 'model-adapter'] as const;
export type AssetKind = (typeof ASSET_KINDS)[number];

export interface AssetEnvelope {
  kind: AssetKind;
  asset_id: string;
  origin_node: string;
  payload_sha: string; // sha256( asset_id + kind + version + origin_node ) 十六进制
  signature: string;   // cosign 同款 DER 签名 base64
  signer_key_id: string;
  provenance: string[];
  version: number;
  created_at: string;
}

export interface Claim {
  task_id: string;
  holder: string;
  version: number;
  lease_until: string;
}

export interface TrustEvent {
  node: string;
  kind: 'contribute' | 'asset_rejected' | 'poison_attempt' | 'forgery_attempt';
  detail: string;
  weight: number;
  at: string;
}

export interface InterceptStats {
  attempts: number;
  intercepted: number;
  false_positives: number;
}

/** 资产载荷完整性哈希（与 Go payload 锚定同算法；TS 消费端可复算）。 */
export function assetPayloadSHA(
  assetId: string,
  kind: AssetKind,
  version: number,
): Promise<string> {
  const data = new TextEncoder().encode(assetId + kind + String(version));
  return crypto.subtle.digest('SHA-256', data as BufferSource).then((d) =>
    [...new Uint8Array(d)].map((b) => b.toString(16).padStart(2, '0')).join(''),
  );
}

/** 验签函数注入面（默认实现：tools/lifecycle 的 cosign 同款 WebCrypto 验签）。 */
export type VerifierFn = (
  payload: string,
  signatureB64: string,
  keyID: string,
) => Promise<Error | null>;

// ===== 资产消费者接收门（与 Go TrustLayer.ReceiveAsset 同结论）=====

export interface ReceiveOptions {
  verify?: VerifierFn;        // 缺省走 tools/lifecycle WebCrypto 验签
  pinnedKeys: string[];       // 钉扎签名钥指纹
  firstSeen: Map<string, string>; // 载荷哈希 → 首发节点（重签刷分指纹；由消费端持有）
  now?: string;
}

export async function receiveAsset(
  asset: AssetEnvelope,
  opts: ReceiveOptions,
): Promise<Error | null> {
  const fail = (msg: string): Error => new Error(`federation: 资产 ${asset.asset_id} ${msg}`);
  const reject = (kind: TrustEvent['kind'], msg: string): Error => {
    void kind; // 事件流由消费端上报面记录；此处仅返回拒绝结论
    return fail(msg);
  };
  // 门 1：完整性（内容哈希不含 origin——重签资产保留同一内容指纹）
  const sha = await assetPayloadSHA(asset.asset_id, asset.kind, asset.version);
  if (sha !== asset.payload_sha) {
    return reject('asset_rejected', '完整性校验失败');
  }
  // 门 2：钉扎 + 验签
  if (!opts.pinnedKeys.includes(asset.signer_key_id)) {
    return reject('forgery_attempt', '签名钥未钉扎');
  }
  const verify = opts.verify ?? defaultVerify;
  const payload = `${asset.asset_id}|${asset.kind}|${asset.payload_sha}|${asset.origin_node}|${asset.version}`;
  if ((await verify(payload, asset.signature, asset.signer_key_id)) !== null) {
    return reject('forgery_attempt', '验签失败');
  }
  // 门 3：溯源回环（自投毒指纹）
  if (asset.provenance.length > 1 && asset.provenance.some((n) => n === asset.origin_node)) {
    return reject('poison_attempt', '溯源链回环');
  }
  // 门 4：重签刷分指纹（他人首发资产原样流通）
  const first = opts.firstSeen.get(asset.payload_sha);
  if (first !== undefined && first !== asset.origin_node) {
    return reject('poison_attempt', '他人资产重签（刷分指纹）');
  }
  if (first === undefined) {
    opts.firstSeen.set(asset.payload_sha, asset.origin_node);
  }
  return null;
}

/** defaultVerify 桥接 tools/lifecycle 的 WebCrypto 验签（载荷拼接口径差异在此封装）。 */
function defaultVerify(payload: string, signatureB64: string, keyID: string): Promise<Error | null> {
  // 键指纹 → 公钥的映射由消费端密钥环提供；契约层先做形态拒绝
  if (!keyID) {
    return Promise.resolve(new Error('lifecycle: 签名钥未钉扎'));
  }
  void payload;
  void signatureB64;
  return Promise.resolve(
    new Error('federation: 需经密钥环解析公钥后调用 verifyCosignSignature（见 tools/lifecycle.ts）'),
  );
}

/** recomputeIntercept 拦截统计复算（与 Go InterceptStats 同口径）。 */
export function recomputeIntercept(events: TrustEvent[]): InterceptStats {
  let attempts = 0;
  let intercepted = 0;
  for (const e of events) {
    if (e.kind !== 'contribute') {
      attempts++;
      intercepted++;
    }
  }
  return { attempts, intercepted, false_positives: 0 };
}
