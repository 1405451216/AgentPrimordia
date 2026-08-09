// Realtime module — Multimodal real-time interaction
// Mirrors Go internal/agent/realtime/

// ===== Session State Machine =====

export enum SessionState {
  Idle = 'idle',
  Listening = 'listening',
  Thinking = 'thinking',
  Speaking = 'speaking',
}

const VALID_TRANSITIONS: Record<SessionState, SessionState[]> = {
  [SessionState.Idle]: [SessionState.Listening],
  [SessionState.Listening]: [SessionState.Thinking, SessionState.Idle],
  [SessionState.Thinking]: [SessionState.Speaking, SessionState.Listening],
  [SessionState.Speaking]: [SessionState.Listening, SessionState.Idle],
};

export interface SessionEvent {
  sessionId: string;
  from: SessionState;
  to: SessionState;
  timestamp: Date;
  reason?: string;
}

export class RealtimeSession {
  readonly id: string;
  state: SessionState = SessionState.Idle;
  readonly createdAt: Date = new Date();
  updatedAt: Date = new Date();
  private listeners: Array<(e: SessionEvent) => void> = [];

  constructor(id: string) {
    this.id = id;
  }

  transitionTo(next: SessionState, reason?: string): void {
    const allowed = VALID_TRANSITIONS[this.state];
    if (!allowed?.includes(next)) {
      throw new Error(`realtime: 非法状态转换 ${this.state} → ${next}`);
    }
    const event: SessionEvent = {
      sessionId: this.id,
      from: this.state,
      to: next,
      timestamp: new Date(),
      reason,
    };
    this.state = next;
    this.updatedAt = new Date();
    for (const fn of this.listeners) fn(event);
  }

  onTransition(fn: (e: SessionEvent) => void): void {
    this.listeners.push(fn);
  }

  get isActive(): boolean {
    return this.state !== SessionState.Idle;
  }
}

// ===== ASR / TTS Adapters =====

export interface ASRAdapter {
  transcribe(audio: Uint8Array): Promise<string>;
  readonly name: string;
}

export interface TTSAdapter {
  synthesize(text: string): Promise<Uint8Array>;
  readonly name: string;
}

export class MockASR implements ASRAdapter {
  readonly name = 'mock-asr';
  async transcribe(audio: Uint8Array): Promise<string> {
    if (audio.length === 0) throw new Error('realtime: 空音频数据');
    return `[transcribed ${audio.length} bytes]`;
  }
}

export class MockTTS implements TTSAdapter {
  readonly name = 'mock-tts';
  async synthesize(text: string): Promise<Uint8Array> {
    if (!text) throw new Error('realtime: 空文本');
    return new TextEncoder().encode(`[audio for: ${text}]`);
  }
}

// ===== Realtime Hub =====

export interface HubConfig {
  asr?: ASRAdapter;
  tts?: TTSAdapter;
  idleTimeoutMs?: number;
}

export class RealtimeHub {
  private sessions = new Map<string, RealtimeSession>();
  private asr: ASRAdapter;
  private tts: TTSAdapter;

  constructor(cfg: HubConfig = {}) {
    this.asr = cfg.asr ?? new MockASR();
    this.tts = cfg.tts ?? new MockTTS();
  }

  createSession(id: string): RealtimeSession {
    const session = new RealtimeSession(id);
    this.sessions.set(id, session);
    return session;
  }

  getSession(id: string): RealtimeSession | undefined {
    return this.sessions.get(id);
  }

  closeSession(id: string): void {
    this.sessions.delete(id);
  }

  async handleAudioInput(sessionId: string, audio: Uint8Array): Promise<{ text: string; audioOut: Uint8Array }> {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`realtime: 会话 ${sessionId} 不存在`);

    if (session.state === SessionState.Idle) {
      session.transitionTo(SessionState.Listening, 'audio input');
    }

    const text = await this.asr.transcribe(audio);
    session.transitionTo(SessionState.Thinking, 'transcription complete');
    session.transitionTo(SessionState.Speaking, 'response ready');

    const responseText = `收到: ${text}`;
    const audioOut = await this.tts.synthesize(responseText);

    session.transitionTo(SessionState.Listening, 'response delivered');
    return { text: responseText, audioOut };
  }

  bargeIn(sessionId: string): void {
    const session = this.sessions.get(sessionId);
    if (!session) throw new Error(`realtime: 会话 ${sessionId} 不存在`);
    if (session.state !== SessionState.Speaking) {
      throw new Error(`realtime: 会话 ${sessionId} 状态 ${session.state}，无法打断`);
    }
    session.transitionTo(SessionState.Listening, 'barge-in');
  }

  get activeSessions(): number {
    return [...this.sessions.values()].filter(s => s.isActive).length;
  }
}

// ===== 真实 ASR/TTS HTTP 适配器（v4.1 双语言对齐，见 adapters.ts） =====

export {
  OpenAIASR,
  OpenAITTS,
  type OpenAIASROptions,
  type OpenAITTSOptions,
} from './adapters.js';
