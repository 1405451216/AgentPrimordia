/**
 * 动态导入模块类型声明。
 *
 * @xenova/transformers 是可选 peer dependency（C-4 WebGPU 推理后端），
 * 仅在用户自行安装后可用。此声明文件消除 tsc 静态分析的模块未找到错误。
 */
declare module '@xenova/transformers' {
  export function pipeline(
    task: string,
    model: string,
    options?: Record<string, unknown>,
  ): Promise<any>;

  export class Pipeline {
    dispose(): Promise<void>;
  }
}
