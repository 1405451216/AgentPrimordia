/**
 * PrivacyRouter 测试
 *
 * 测试 PII 检测、PII 脱敏、隐私优先路由策略
 */

import { describe, it, expect } from 'vitest';
import {
  detectPII,
  hasPII,
  redactPII,
  PrivacyRouter,
  type PIIType,
} from '../../src/llm/privacy-router.js';

describe('PII Detection', () => {
  describe('detectPII', () => {
    it('should detect email addresses', () => {
      const text = 'Contact me at john@example.com for details';
      const detections = detectPII(text);

      expect(detections.length).toBeGreaterThan(0);
      expect(detections[0].type).toBe('email');
      expect(detections[0].value).toBe('john@example.com');
    });

    it('should detect phone numbers (Chinese format)', () => {
      const text = 'Call me at 13812345678';
      const detections = detectPII(text);

      const phones = detections.filter(d => d.type === 'phone');
      expect(phones.length).toBeGreaterThan(0);
    });

    it('should detect IP addresses', () => {
      const text = 'Server is at 192.168.1.100';
      const detections = detectPII(text);

      const ips = detections.filter(d => d.type === 'ip_address');
      expect(ips.length).toBeGreaterThan(0);
    });

    it('should detect multiple PII types in one text', () => {
      const text = 'Email: john@example.com, Phone: 13812345678, IP: 10.0.0.1';
      const detections = detectPII(text);

      expect(detections.length).toBeGreaterThanOrEqual(3);
      const types = new Set(detections.map(d => d.type));
      expect(types.has('email')).toBe(true);
      expect(types.has('phone')).toBe(true);
      expect(types.has('ip_address')).toBe(true);
    });

    it('should return empty array for text without PII', () => {
      const text = 'This is a normal message without any PII';
      const detections = detectPII(text);

      expect(detections.length).toBe(0);
    });

    it('should sort detections by position', () => {
      const text = 'IP 10.0.0.1 and email test@example.com';
      const detections = detectPII(text);

      for (let i = 1; i < detections.length; i++) {
        expect(detections[i].start).toBeGreaterThanOrEqual(detections[i - 1].start);
      }
    });
  });

  describe('hasPII', () => {
    it('should return true for text with PII', () => {
      expect(hasPII('Email: test@example.com')).toBe(true);
    });

    it('should return false for text without PII', () => {
      expect(hasPII('No sensitive data here')).toBe(false);
    });
  });
});

describe('PII Redaction', () => {
  describe('redactPII', () => {
    it('should redact email addresses', () => {
      const text = 'Contact john@example.com for info';
      const { redacted, detections } = redactPII(text);

      expect(redacted).toContain('[EMAIL]');
      expect(redacted).not.toContain('john@example.com');
      expect(detections.length).toBeGreaterThan(0);
    });

    it('should redact multiple PII types', () => {
      const text = 'Email: a@b.com IP: 1.2.3.4';
      const { redacted, detections } = redactPII(text);

      expect(redacted).toContain('[EMAIL]');
      expect(redacted).toContain('[IP]');
      expect(detections.length).toBeGreaterThanOrEqual(2);
    });

    it('should not modify text without PII', () => {
      const text = 'No PII here';
      const { redacted, detections } = redactPII(text);

      expect(redacted).toBe(text);
      expect(detections.length).toBe(0);
    });

    it('should handle overlapping PII correctly', () => {
      const text = 'a@b.com 13800001111';
      const { redacted } = redactPII(text);

      // 不应崩溃，应正确替换
      expect(redacted).not.toContain('a@b.com');
    });
  });
});

describe('PrivacyRouter', () => {
  describe('route', () => {
    it('should route to remote when no PII', () => {
      const router = new PrivacyRouter({
        enableLocal: true,
        allowRedactedRemote: true,
      });

      const result = router.route('Hello, how are you?');

      expect(result.decision).toBe('remote');
      expect(result.piiDetected.length).toBe(0);
    });

    it('should route to local when PII detected and local available', () => {
      const router = new PrivacyRouter({
        enableLocal: true,
        allowRedactedRemote: true,
      });

      // 模拟本地可用
      // @ts-expect-error - accessing private field for testing
      router.localAvailable = true;

      const result = router.route('My email is test@example.com');

      expect(result.decision).toBe('local');
      expect(result.piiDetected.length).toBeGreaterThan(0);
    });

    it('should route to redacted_remote when PII but local unavailable', () => {
      const router = new PrivacyRouter({
        enableLocal: true,
        allowRedactedRemote: true,
      });

      // localAvailable 默认为 false
      const result = router.route('My email is test@example.com');

      expect(result.decision).toBe('redacted_remote');
      expect(result.processedInput).toContain('[EMAIL]');
      expect(result.processedInput).not.toContain('test@example.com');
    });

    it('should route to local when redaction not allowed', () => {
      const router = new PrivacyRouter({
        enableLocal: true,
        allowRedactedRemote: false,
      });

      const result = router.route('My email is test@example.com');

      // 本地不可用且不允许脱敏 → 回退到 local 决策（实际会拒绝）
      expect(result.decision).toBe('local');
    });
  });

  describe('getStats', () => {
    it('should return config stats', () => {
      const router = new PrivacyRouter({
        enableLocal: true,
        allowRedactedRemote: false,
      });

      const stats = router.getStats();

      expect(stats.enableLocal).toBe(true);
      expect(stats.allowRedactedRemote).toBe(false);
      expect(typeof stats.localAvailable).toBe('boolean');
    });
  });

  describe('default config values', () => {
    it('should apply default values when not specified', () => {
      const router = new PrivacyRouter({});

      const stats = router.getStats();
      expect(stats.enableLocal).toBe(true);
      expect(stats.allowRedactedRemote).toBe(true);
    });
  });
});
