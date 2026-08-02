/**
 * v3.5 Studio Web UI — A2A Interop
 *
 * 协议兼容性状态 + 生态客户端接入示例
 */
import { useState, useEffect } from 'react';

interface InteropStatus {
  mode: string;
  agentCardExposed: boolean;
  agentCardUrl: string;
  supportedMethods: string[];
  ioModes: { input: string[]; output: string[] };
}

export function A2AInterop() {
  const [status, setStatus] = useState<InteropStatus | null>(null);

  useEffect(() => {
    const refresh = async () => {
      try {
        const res = await fetch('/api/v1/a2a/interop/status');
        if (res.ok) setStatus(await res.json());
      } catch { /* 忽略 */ }
    };
    refresh();
  }, []);

  return (
    <div style={{ padding: 24 }}>
      <h1>🌐 A2A 协议互操作</h1>

      {status && (
        <>
          <div style={{ background: '#f9fafb', borderRadius: 8, padding: 16, marginBottom: 16 }}>
            <h3>兼容性状态</h3>
            <table>
              <tbody>
                <tr><td style={{ padding: 4, fontWeight: 'bold' }}>模式</td><td>{status.mode}</td></tr>
                <tr><td style={{ padding: 4, fontWeight: 'bold' }}>Agent Card</td><td>{status.agentCardExposed ? `✅ ${status.agentCardUrl}` : '❌ 未暴露'}</td></tr>
                <tr><td style={{ padding: 4, fontWeight: 'bold' }}>支持方法</td><td>{status.supportedMethods.join(', ')}</td></tr>
                <tr><td style={{ padding: 4, fontWeight: 'bold' }}>输入模式</td><td>{status.ioModes.input.join(', ')}</td></tr>
                <tr><td style={{ padding: 4, fontWeight: 'bold' }}>输出模式</td><td>{status.ioModes.output.join(', ')}</td></tr>
              </tbody>
            </table>
          </div>

          <div style={{ background: '#f0fdf4', borderRadius: 8, padding: 16 }}>
            <h3>接入示例</h3>
            <pre style={{ background: '#1e293b', color: '#e2e8f0', padding: 12, borderRadius: 4, overflow: 'auto', fontSize: 13 }}>
{`# 获取 Agent Card
curl ${status.agentCardUrl}

# 发送任务（JSON-RPC）
curl -X POST http://localhost:8080/a2a/v1 \\
  -H "Content-Type: application/json" \\
  -d '{"jsonrpc":"2.0","method":"tasks/send","id":1,
       "params":{"message":{"role":"user","parts":[{"type":"text","text":"hello"}]}}}

# 查询任务状态
curl -X POST http://localhost:8080/a2a/v1 \\
  -d '{"jsonrpc":"2.0","method":"tasks/get","id":2,
       "params":{"taskId":"task-xxx"}}'`}
            </pre>
          </div>
        </>
      )}

      {!status && <p style={{ color: '#6b7280' }}>加载互操作状态...</p>}
    </div>
  );
}
