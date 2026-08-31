/**
 * pipeline.test.ts — 蒸馏管道工件消费端测试（矩阵 #2 对等）。
 *
 * Go 权威夹具 agentprimordia/internal/agent/learning/pipeline/testdata/
 * dataset_fixture.json（ap-dataset-v1 数据集 + 影子报告）：
 *   - TS 解析同一 JSONL → 样例逐条一致（格式契约）；
 *   - TS 互证 sha256/字节数/行数/manifest_id → 与 Go VerifyDataset 同结论；
 *   - TS 复算影子判据 → 与 Go ShadowReport.Passed 同结论（不信任生产方）。
 */
import { describe, expect, it } from 'vitest';
import { readFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { fileURLToPath } from 'node:url';
import {
  parseDataset,
  verifyDataset,
  recomputeVerdict,
  wilsonLower,
  FORMAT_VERSION,
  type DatasetManifest,
  type ShadowReport,
} from '../../learning/pipeline.js';

const __dirname = dirname(fileURLToPath(import.meta.url));
const FIXTURES_PATH = resolve(
  __dirname,
  '../../../../../agentprimordia/internal/agent/learning/pipeline/testdata/dataset_fixture.json',
);

interface DatasetFixture {
  manifest: DatasetManifest;
  jsonl: string;
  report: ShadowReport;
}

const FX = JSON.parse(readFileSync(FIXTURES_PATH, 'utf-8')) as DatasetFixture;

describe('ap-dataset-v1 工件消费端（Go 权威夹具对账）', () => {
  it('解析 Go 产出的 JSONL：样例数与域标签与清单一致', () => {
    const examples = parseDataset(FX.jsonl);
    expect(examples).toHaveLength(FX.manifest.count);
    for (const ex of examples) {
      expect(ex.domain).toBe(FX.manifest.domain);
      expect(ex.messages.length).toBeGreaterThan(0);
      expect(ex.weight).toBeGreaterThanOrEqual(0);
      expect(ex.weight).toBeLessThanOrEqual(1);
    }
    // 聊天格式契约：assistant 工具调用与 tool 结果按 call-N 成对关联
    const first = examples[0];
    const callMsg = first.messages.find((m) => m.role === 'assistant' && m.tool_calls);
    const toolMsg = first.messages.find((m) => m.role === 'tool');
    expect(callMsg).toBeDefined();
    expect(toolMsg).toBeDefined();
    const callID = JSON.parse(callMsg!.tool_calls!)[0].id as string;
    expect(toolMsg!.tool_call_id).toBe(callID);
    expect(toolMsg!.name).toBe(JSON.parse(callMsg!.tool_calls!)[0].name);
  });

  it('互证：sha256/字节数/行数/manifest_id 与 Go VerifyDataset 同结论', async () => {
    expect(await verifyDataset(FX.jsonl, FX.manifest)).toBeNull();
    expect(FX.manifest.format_version).toBe(FORMAT_VERSION);
    // 篡改一个字节 → 互证拒绝（与 Go 黄金门同语义）
    const tampered = FX.jsonl.replace('hello', 'hellO');
    const err = await verifyDataset(tampered, FX.manifest);
    expect(err).not.toBeNull();
    expect(err!.message).toContain('sha256');
  });

  it('影子判据复算：与 Go ShadowReport.Passed 同结论（不信任生产方字段）', () => {
    // 夹具形态：旗舰 5/5、影子 4/5 → 点 0.80 < 0.85，判据不过
    expect(recomputeVerdict(FX.report)).toBe(false);
    expect(FX.report.passed).toBe(false);
    // Wilson 下界双线同值（1e-9 容差——浮点跨语言允许末位漂移）
    const lo = wilsonLower(FX.report.shadow_success, FX.report.n);
    expect(Math.abs(lo - FX.report.shadow_wilson_lo)).toBeLessThan(1e-9);
  });

  it('判据正例：满分配对且 n 达标时复算通过', () => {
    const rep: ShadowReport = {
      ...FX.report,
      n: 16,
      champion_success: 16,
      shadow_success: 16,
      champion_rate: 1,
      shadow_rate: 1,
    };
    expect(recomputeVerdict(rep)).toBe(true);
    // n=16 全对：Wilson 下界 = 1/(1+z²/n) ≈ 0.8065 ≥ 0.80
    expect(wilsonLower(16, 16)).toBeGreaterThan(0.8);
  });
});
