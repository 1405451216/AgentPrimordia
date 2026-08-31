/**
 * lifecycle.test.ts — 工具生命周期 TS 协议对等测试（矩阵 #3）。
 *
 * Go 权威夹具 agentprimordia/internal/tools/lifecycle/testdata/trust_fixture.json
 * （ECDSA P-256 / SHA-256 / ASN.1 DER 签名）：TS 经 WebCrypto 验证同一签名
 * ——注册客户端本地验签门与 Go VerifyCandidate 同结论。
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import { createHash } from 'node:crypto';
import {
  derToRaw,
  hexToBytes,
  verifyCosignSignature,
  InMemoryRegistrationClient,
  STAGES,
  type ToolPackage,
} from '../lifecycle.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const FIXTURES_PATH = resolve(
  __dirname,
  '../../../../../agentprimordia/internal/tools/lifecycle/testdata/trust_fixture.json',
);

interface TrustFixture {
  payload: string;
  payload_sha256: string;
  signature_der_b64: string;
  pub_uncompressed_b64: string;
  note: string;
}

const FX = JSON.parse(readFileSync(FIXTURES_PATH, 'utf-8')) as TrustFixture;

describe('签名信任链跨语言对账（Go 权威夹具）', () => {
  it('DER→raw 转换 + WebCrypto 验签与 Go 同结论（合法签名通过）', async () => {
    // 摘要一致性：Node crypto 与 Go payload_sha256 对账
    const digest = createHash('sha256').update(FX.payload, 'utf-8').digest('hex');
    expect(digest).toBe(FX.payload_sha256);
    // 验签
    expect(await verifyCosignSignature(FX.payload, FX.signature_der_b64, FX.pub_uncompressed_b64)).toBeNull();
  });

  it('篡改 payload / 坏 DER / 坏公钥 → 拒绝（与 Go 守卫同语义）', async () => {
    const err = await verifyCosignSignature(FX.payload + 'x', FX.signature_der_b64, FX.pub_uncompressed_b64);
    expect(err).not.toBeNull();
    expect(err!.message).toContain('签名验证失败');
    // 坏 DER（非序列头）
    const badDer = Buffer.from('not-der').toString('base64');
    expect((await verifyCosignSignature(FX.payload, badDer, FX.pub_uncompressed_b64))!.message).toContain('DER');
    // 压缩公钥形式拒绝
    const compressed = Buffer.from('02' + FX.pub_uncompressed_b64.slice(2, 66), 'hex').toString('base64');
    expect((await verifyCosignSignature(FX.payload, FX.signature_der_b64, compressed))!.message).toContain('非压缩');
  });

  it('derToRaw：DER 签名转 64 字节 raw（r||s 定长对齐）', () => {
    const der = Uint8Array.from(atob(FX.signature_der_b64), (c) => c.charCodeAt(0));
    const raw = derToRaw(der);
    expect(raw).toHaveLength(64);
  });
});

describe('注册客户端契约（本地验签门 + 确定性内存实现）', () => {
  it('非注册态拒绝、重复注册拒绝、清单确定', async () => {
    const client = new InMemoryRegistrationClient();
    const pkg: ToolPackage = {
      id: 'gap-missing_tool-code_exec',
      name: 'code_exec',
      domain: 'missing_tool',
      description: '缺口闭合工件',
      artifact_sha256: 'ab'.repeat(32),
      signer_key_fingerprint: 'pinned-1',
      stage: 'rehearsed',
    };
    const rej = await client.submit(pkg);
    expect(rej.accepted).toBe(false);
    const ok = await client.submit({ ...pkg, stage: 'signed_registered' });
    expect(ok.accepted).toBe(true);
    const dup = await client.submit({ ...pkg, stage: 'signed_registered' });
    expect(dup.accepted).toBe(false);
    const list = await client.list();
    expect(list).toHaveLength(1);
    expect(list[0].stage).toBe('signed_registered');
    // 阶段集合封闭且有序（与 Go stageOrder 对齐）
    expect(STAGES).toEqual([
      'gap_detected',
      'generated',
      'rehearsed',
      'adversarial_tested',
      'signed_registered',
      'retired',
    ]);
  });

  it('hexToBytes 与 Go hex.EncodeToString 互逆', () => {
    const bytes = hexToBytes('fb7d0546');
    expect([...bytes]).toEqual([0xfb, 0x7d, 0x05, 0x46]);
  });
});
