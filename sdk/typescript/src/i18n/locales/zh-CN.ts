/**
 * 中文语言包 (简体)
 * Simplified Chinese locale strings for the AgentPrimordia SDK.
 */

export const zhCN = {
  // 通用
  'common.yes': '是',
  'common.no': '否',
  'common.ok': '确定',
  'common.cancel': '取消',
  'common.loading': '加载中...',
  'common.error': '错误',
  'common.success': '成功',
  'common.warning': '警告',

  // 参数校验
  'validation.required': '字段 "{field}" 为必填项',
  'validation.type': '字段 "{field}" 必须是 {type} 类型',
  'validation.range': '值必须在 {min} 到 {max} 之间',
  'validation.format': '字段 "{field}" 格式无效',

  // 超时
  'timeout.operation': '操作在 {seconds} 秒后超时',
  'timeout.connect': '连接在 {seconds} 秒后超时',
  'timeout.request': '请求在 {seconds} 秒后超时',

  // 权限
  'permission.denied': '权限不足',
  'permission.forbidden': '访问被拒绝',
  'permission.scope': '权限范围不足：需要 {scope}',
  'permission.auth': '需要身份认证',

  // Agent 相关
  'agent.running': 'Agent 已在运行中',
  'agent.stopped': 'Agent 已停止',
  'agent.maxTurns': '已达到最大轮数 ({count})',
  'agent.noToolkit': 'Agent 未配置工具集',
  'agent.initFailed': 'Agent 初始化失败：{reason}',

  // Tool 相关
  'tool.notFound': '工具 "{name}" 未找到',
  'tool.execution': '工具执行失败：{reason}',
  'tool.invalidConfig': '工具配置无效',
  'tool.confirmDenied': '工具确认被用户拒绝',

  // LLM 相关
  'llm.callFailed': 'LLM 调用失败：{reason}',
  'llm.apiKey': 'Provider "{provider}" 需要 API Key',
  'llm.emptyResponse': 'LLM 返回空响应',
  'llm.parseFailed': '解析 LLM 响应失败',
  'llm.retriesExhausted': '已耗尽所有重试次数',
  'llm.circuitOpen': 'Provider "{provider}" 的熔断器已打开',
  'llm.rateLimited': '请求频率超限，请在 {seconds} 秒后重试',

  // Memory 相关
  'memory.episodeNotFound': '片段 "{id}" 不存在',
  'memory.invalidImportance': '重要度必须在 0 到 1 之间',
  'memory.emptyId': 'ID 不能为空',
  'memory.emptyContent': '内容不能为空',

  // Security 相关
  'security.commandBlocked': '命令被安全策略阻止',
  'security.accessDenied': '访问被拒绝',
  'security.pathTraversal': '检测到路径遍历攻击',

  // WebSocket 相关
  'ws.connectFailed': 'WebSocket 连接失败：{reason}',
  'ws.maxReconnect': '已达到最大重连次数 ({count})',
  'ws.timeout': 'WebSocket 在 {seconds} 秒后超时',

  // HTTP 相关
  'http.error': 'HTTP 错误 {status}：{message}',
  'http.serverError': '服务器错误：{status}',
} as const;

export type ZhCNLocale = typeof zhCN;