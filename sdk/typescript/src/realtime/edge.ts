// Realtime Edge — 浏览器/边缘实时推理链路（v3.6）
// Mirrors Go internal/agent/realtime/integration.go EdgeInference
// 复用 edge/ + webgpu-provider.ts 的 WebGPU 推理能力。

export interface EdgeInference {
  infer(input: Uint8Array): Promise<Uint8Array>;
  available(): boolean;
}

// WebGPU 边缘推理适配器（浏览器侧）
export class WebGPUEdgeInference implements EdgeInference {
  private device: unknown | null = null;
  private checked = false;

  available(): boolean {
    if (!this.checked) {
      this.checked = true;
      // 浏览器环境检测 WebGPU
      const nav = (globalThis as { navigator?: { gpu?: unknown } }).navigator;
      this.device = nav?.gpu ?? null;
    }
    return this.device !== null;
  }

  async infer(input: Uint8Array): Promise<Uint8Array> {
    if (!this.available()) {
      throw new Error('realtime edge: WebGPU 不可用');
    }
    // 真实实现应经 webgpu-provider 运行模型；此处为链路占位
    return input;
  }
}

// 回退包装：边缘不可用时回退云端回调
export class EdgeWithFallback implements EdgeInference {
  constructor(
    private edge: EdgeInference,
    private cloudFallback: (input: Uint8Array) => Promise<Uint8Array>,
  ) {}

  available(): boolean {
    return this.edge.available();
  }

  async infer(input: Uint8Array): Promise<Uint8Array> {
    if (this.edge.available()) {
      try {
        return await this.edge.infer(input);
      } catch {
        // 边缘推理失败，回退云端
      }
    }
    return this.cloudFallback(input);
  }
}
