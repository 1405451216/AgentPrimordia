/**
 * AgentPrimordia Studio 后端 API 客户端
 *
 * 提供对 Studio REST 接口的类型安全封装。所有方法在不可达时 rejected，
 * 由调用方决定降级策略。
 */

import type { RunSummary } from './messages.js';

/** API 请求异常 */
export class ApiError extends Error {
    constructor(
        message: string,
        public readonly status?: number,
    ) {
        super(message);
        this.name = 'ApiError';
    }
}

/** Studio 健康响应 */
export interface HealthResponse {
    status: string;
    version?: string;
}

/** Studio 运行列表响应（简化） */
export interface RunsResponse {
    runs: RunSummary[];
}

/** Studio API 客户端 */
export class StudioApiClient {
    constructor(private readonly baseUrl: string) {}

    /** 拼接完整 URL */
    private url(path: string): string {
        return `${this.baseUrl.replace(/\/$/, '')}${path}`;
    }

    /** 带超时与错误处理的 fetch 封装 */
    private async request<T>(path: string, init?: RequestInit): Promise<T> {
        const controller = new AbortController();
        const timeout = setTimeout(() => controller.abort(), 5000);
        try {
            const resp = await fetch(this.url(path), {
                ...init,
                signal: controller.signal,
                headers: { 'Content-Type': 'application/json', ...init?.headers },
            });
            if (!resp.ok) {
                throw new ApiError(`API ${resp.status}: ${resp.statusText}`, resp.status);
            }
            return (await resp.json()) as T;
        } catch (err) {
            if (err instanceof ApiError) throw err;
            throw new ApiError(
                `Network error: ${err instanceof Error ? err.message : String(err)}`,
            );
        } finally {
            clearTimeout(timeout);
        }
    }

    /** 检查 Studio 后端是否可达 */
    async health(): Promise<HealthResponse> {
        return this.request<HealthResponse>('/api/health');
    }

    /** 获取最近运行列表 */
    async recentRuns(limit = 10): Promise<RunSummary[]> {
        const data = await this.request<RunsResponse>(`/api/runs?limit=${limit}`);
        return data.runs ?? [];
    }

    /** 获取单次运行的 trace 事件 */
    async getTraces(runId: string): Promise<unknown[]> {
        return this.request<unknown[]>(`/api/runs/${encodeURIComponent(runId)}/traces`);
    }
}

/** 默认客户端实例（使用默认 studio URL） */
export const defaultClient = new StudioApiClient('http://localhost:8765');
