/**
 * protocol.test.ts — 联邦资产消费者接收门测试（矩阵 #5）。
 *
 * 验签算法跨语言一致性已由 tools/__tests__/lifecycle.test.ts（Go 权威
 * ECDSA 夹具）覆盖；本文件验证接收门四道闸的 TS 本端语义与 Go
 * TrustLayer.ReceiveAsset 同结论。
 */
import { describe, expect, it } from 'vitest';
import { createHash } from 'node:crypto';
import {
  receiveAsset,
  recomputeIntercept,
  assetPayloadSHA,
  ASSET_KINDS,
  type AssetEnvelope,
  type TrustEvent,
} from '../protocol.js';

const okVerify = async (): Promise<null> => null;
const badVerify = async (): Promise<Error> => new Error('签名不匹配');

function envelope(
  id: string,
  origin: string,
  version: number,
  signer: string,
): AssetEnvelope {
  const sha = createHash('sha256')
    .update(id + 'skill-card' + String(version))
    .digest('hex');
  return {
    kind: 'skill-card',
    asset_id: id,
    origin_node: origin,
    payload_sha: sha,
    signature: 'sig',
    signer_key_id: signer,
    provenance: [origin],
    version,
    created_at: '2026-09-01T00:00:00Z',
  };
}

describe('联邦资产接收门（与 Go TrustLayer.ReceiveAsset 同结论）', () => {
  it('合法资产通过；四道闸逐一拒绝（完整性/钉扎/验签/回环/刷分指纹）', async () => {
    const firstSeen = new Map<string, string>();
    const opts = { verify: okVerify, pinnedKeys: ['key-1'], firstSeen };
    // 合法
    expect(await receiveAsset(envelope('a-1', 'node-a', 1, 'key-1'), opts)).toBeNull();
    // 门 1 完整性
    const tampered = envelope('a-2', 'node-a', 1, 'key-1');
    tampered.payload_sha = 'deadbeef';
    expect((await receiveAsset(tampered, opts))!.message).toContain('完整性');
    // 门 2 钉扎
    expect((await receiveAsset(envelope('a-3', 'node-a', 1, 'key-evil'), opts))!.message).toContain('未钉扎');
    // 门 2b 验签
    expect(
      (await receiveAsset(envelope('a-4', 'node-a', 1, 'key-1'), { ...opts, verify: badVerify }))!.message,
    ).toContain('验签失败');
    // 门 3 溯源回环
    const loop = envelope('a-5', 'node-a', 1, 'key-1');
    loop.provenance = ['node-b', 'node-a'];
    expect((await receiveAsset(loop, opts))!.message).toContain('回环');
    // 门 4 重签刷分指纹（他人首发）
    const first = envelope('shared', 'node-good', 1, 'key-1');
    expect(await receiveAsset(first, opts)).toBeNull();
    const replay = envelope('shared', 'node-bad', 1, 'key-1');
    replay.payload_sha = first.payload_sha;
    expect((await receiveAsset(replay, opts))!.message).toContain('刷分');
    // 首发节点自身重放：幂等允许
    expect(await receiveAsset(first, opts)).toBeNull();
  });

  it('拦截统计复算与 Go InterceptStats 同口径', () => {
    const events: TrustEvent[] = [
      { node: 'n1', kind: 'contribute', detail: 'ok', weight: 1, at: 't' },
      { node: 'n2', kind: 'poison_attempt', detail: 'x', weight: -1, at: 't' },
      { node: 'n2', kind: 'forgery_attempt', detail: 'y', weight: -1, at: 't' },
    ];
    expect(recomputeIntercept(events)).toEqual({ attempts: 2, intercepted: 2, false_positives: 0 });
  });

  it('资产三形态封闭集合与载荷哈希确定性', async () => {
    expect(ASSET_KINDS).toEqual(['skill-card', 'tool-package', 'model-adapter']);
    const sha1 = await assetPayloadSHA('a-1', 'skill-card', 1);
    const sha2 = await assetPayloadSHA('a-1', 'skill-card', 1);
    const sha3 = await assetPayloadSHA('a-1', 'tool-package', 1);
    expect(sha1).toBe(sha2);
    expect(sha1).not.toBe(sha3);
    // Node crypto 复算一致（Go 同算法口径：内容哈希不含 origin）
    const expectSha = createHash('sha256').update('a-1' + 'skill-card' + '1').digest('hex');
    expect(sha1).toBe(expectSha);
  });
});
