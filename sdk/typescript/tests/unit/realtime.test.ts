import { describe, it, expect } from 'vitest';
import {
  SessionState, RealtimeSession, RealtimeHub, MockASR, MockTTS,
} from '../../src/realtime/index.js';

describe('realtime session state machine (v3.6)', () => {
  it('walks the full lifecycle', () => {
    const s = new RealtimeSession('s1');
    expect(s.state).toBe(SessionState.Idle);
    s.transitionTo(SessionState.Listening, 'start');
    s.transitionTo(SessionState.Thinking, 'asr');
    s.transitionTo(SessionState.Speaking, 'llm');
    s.transitionTo(SessionState.Listening, 'tts');
    s.transitionTo(SessionState.Idle, 'end');
    expect(s.state).toBe(SessionState.Idle);
  });

  it('rejects illegal transitions', () => {
    const s = new RealtimeSession('s2');
    expect(() => s.transitionTo(SessionState.Speaking, 'bad')).toThrow();
    expect(s.state).toBe(SessionState.Idle);
  });

  it('emits transition events', () => {
    const s = new RealtimeSession('s3');
    const events: string[] = [];
    s.onTransition(e => events.push(`${e.from}->${e.to}`));
    s.transitionTo(SessionState.Listening, 'x');
    expect(events).toEqual(['idle->listening']);
  });

  it('isActive reflects non-idle', () => {
    const s = new RealtimeSession('s4');
    expect(s.isActive).toBe(false);
    s.transitionTo(SessionState.Listening, 'x');
    expect(s.isActive).toBe(true);
  });
});

describe('RealtimeHub', () => {
  it('handles audio input end-to-end with mocks', async () => {
    const hub = new RealtimeHub();
    hub.createSession('h1');
    const { text, audioOut } = await hub.handleAudioInput('h1', new Uint8Array([1, 2, 3]));
    expect(text).toContain('收到');
    expect(audioOut.length).toBeGreaterThan(0);
    expect(hub.getSession('h1')!.state).toBe(SessionState.Listening);
  });

  it('barge-in only from speaking', async () => {
    const hub = new RealtimeHub();
    const s = hub.createSession('h2');
    expect(() => hub.bargeIn('h2')).toThrow();
    s.transitionTo(SessionState.Listening, 'x');
    s.transitionTo(SessionState.Thinking, 'x');
    s.transitionTo(SessionState.Speaking, 'x');
    hub.bargeIn('h2');
    expect(s.state).toBe(SessionState.Listening);
  });

  it('counts active sessions', () => {
    const hub = new RealtimeHub();
    const s = hub.createSession('h3');
    expect(hub.activeSessions).toBe(0);
    s.transitionTo(SessionState.Listening, 'x');
    expect(hub.activeSessions).toBe(1);
  });
});

describe('MockASR / MockTTS', () => {
  it('ASR rejects empty audio', async () => {
    const asr = new MockASR();
    await expect(asr.transcribe(new Uint8Array())).rejects.toThrow();
  });

  it('TTS rejects empty text', async () => {
    const tts = new MockTTS();
    await expect(tts.synthesize('')).rejects.toThrow();
  });
});
