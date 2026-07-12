/**
 * 日本語言語パッケージ
 * Japanese locale strings for the AgentPrimordia SDK.
 */

export const ja = {
  // 一般
  'common.yes': 'はい',
  'common.no': 'いいえ',
  'common.ok': 'OK',
  'common.cancel': 'キャンセル',
  'common.loading': '読み込み中...',
  'common.error': 'エラー',
  'common.success': '成功',
  'common.warning': '警告',

  // パラメータ検証
  'validation.required': 'フィールド "{field}" は必須です',
  'validation.type': 'フィールド "{field}" は {type} 型である必要があります',
  'validation.range': '値は {min} から {max} の間である必要があります',
  'validation.format': 'フィールド "{field}" の形式が無効です',

  // タイムアウト
  'timeout.operation': '操作が {seconds} 秒後にタイムアウトしました',
  'timeout.connect': '接続が {seconds} 秒後にタイムアウトしました',
  'timeout.request': 'リクエストが {seconds} 秒後にタイムアウトしました',

  // 権限
  'permission.denied': '権限が拒否されました',
  'permission.forbidden': 'アクセスが禁止されています',
  'permission.scope': '権限範囲が不足しています：{scope} が必要です',
  'permission.auth': '認証が必要です',

  // Agent 関連
  'agent.running': 'Agent は既に実行中です',
  'agent.stopped': 'Agent が停止しました',
  'agent.maxTurns': '最大ターン数 ({count}) を超えました',
  'agent.noToolkit': 'Agent にツールキットが設定されていません',
  'agent.initFailed': 'Agent の初期化に失敗しました：{reason}',

  // Tool 関連
  'tool.notFound': 'ツール "{name}" が見つかりません',
  'tool.execution': 'ツールの実行に失敗しました：{reason}',
  'tool.invalidConfig': 'ツールの設定が無効です',
  'tool.confirmDenied': 'ツールの確認がユーザーによって拒否されました',

  // LLM 関連
  'llm.callFailed': 'LLM 呼び出しに失敗しました：{reason}',
  'llm.apiKey': 'プロバイダ "{provider}" には API キーが必要です',
  'llm.emptyResponse': 'LLM が空の応答を返しました',
  'llm.parseFailed': 'LLM 応答の解析に失敗しました',
  'llm.retriesExhausted': 'すべての再試行回数が使い果たされました',
  'llm.circuitOpen': 'プロバイダ "{provider}" の回路ブレーカーが開いています',
  'llm.rateLimited': 'レート制限を超えました。{seconds} 秒後に再試行してください',

  // Memory 関連
  'memory.episodeNotFound': 'エピソード "{id}" が見つかりません',
  'memory.invalidImportance': '重要度は 0 から 1 の間である必要があります',
  'memory.emptyId': 'ID は空にできません',
  'memory.emptyContent': 'コンテンツは空にできません',

  // Security 関連
  'security.commandBlocked': 'コマンドがセキュリティポリシーによってブロックされました',
  'security.accessDenied': 'アクセスが拒否されました',
  'security.pathTraversal': 'パストラバーサルが検出されました',

  // WebSocket 関連
  'ws.connectFailed': 'WebSocket 接続に失敗しました：{reason}',
  'ws.maxreconnect': '最大再接続回数 ({count}) に達しました',
  'ws.timeout': 'WebSocket が {seconds} 秒後にタイムアウトしました',

  // HTTP 関連
  'http.error': 'HTTP エラー {status}：{message}',
  'http.serverError': 'サーバーエラー：{status}',
} as const;

export type JaLocale = typeof ja;