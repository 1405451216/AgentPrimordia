/**
 * WebGPU 能力检测工具
 *
 * 提供独立的 WebGPU 可用性探测功能，支持分级评估：
 * - none: 不可用
 * - basic: 可用但计算能力有限
 * - full: 可用且计算能力充足（适合 LLM 推理）
 */

/** WebGPU 适配器信息（与 DOM GPUAdapterInfo 对齐） */
interface GPUAdapterInfo {
  description?: string;
  vendor?: string;
  architecture?: string;
  device?: string;
}

/** WebGPU 设备限制（与 DOM GPULimits 对齐） */
interface GPULimits {
  maxBufferSize?: number;
  maxComputeWorkgroupsPerDimension?: number;
  maxStorageBufferBindingSize?: number;
  maxComputeWorkgroupStorageSize?: number;
}

/** navigator.gpu 最小接口 */
interface GPUNavigator {
  requestAdapter(): Promise<GPUAdapter | null>;
}

/** GPUAdapter 最小接口 */
interface GPUAdapter {
  requestDevice(): Promise<GPUDevice>;
  info?: GPUAdapterInfo;
  features: Set<string>;
  requestAdapterInfo?(): Promise<GPUAdapterInfo>;
}

/** GPUDevice 最小接口 */
interface GPUDevice {
  limits: GPULimits;
}

/** 检测结果 */
export interface WebGPUDetectResult {
  supported: boolean;
  tier: 'none' | 'basic' | 'full';
  adapterName?: string;
  maxBufferSize?: number;
  maxComputeWorkgroups?: number;
}

/**
 * 检测当前环境的 WebGPU 可用性并分级。
 *
 * 分级逻辑：
 * - 无 navigator.gpu 或 requestAdapter 返回 null → none
 * - maxComputeWorkgroupsPerDimension < 65535 或 maxBufferSize < 256MB → basic
 * - 否则 → full（适合运行量化 LLM）
 *
 * 用法：
 *   const result = await detectWebGPU();
 *   if (result.supported && result.tier === 'full') {
 *     // 可以加载 7B+ 模型
 *   }
 */
export async function detectWebGPU(): Promise<WebGPUDetectResult> {
  // 非浏览器环境直接返回 none
  if (typeof navigator === 'undefined' || !(navigator as any).gpu) {
    return { supported: false, tier: 'none' };
  }

  try {
    const nav = (navigator as any).gpu as unknown as GPUNavigator;
    const adapter = await nav.requestAdapter();
    if (!adapter) return { supported: false, tier: 'none' };

    const device = await adapter.requestDevice();
    const limits = device.limits;

    // 计算能力分级
    const maxWG = limits.maxComputeWorkgroupsPerDimension ?? 0;
    const maxBuf = limits.maxBufferSize ?? 0;

    let tier: 'basic' | 'full' = 'basic';
    if (maxWG >= 65535 && maxBuf >= 256 * 1024 * 1024) {
      tier = 'full';
    }

    return {
      supported: true,
      tier,
      adapterName: adapter.info?.description,
      maxBufferSize: maxBuf,
      maxComputeWorkgroups: maxWG,
    };
  } catch {
    return { supported: false, tier: 'none' };
  }
}