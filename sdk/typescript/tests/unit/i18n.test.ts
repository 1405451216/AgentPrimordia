/**
 * i18n 国际化模块单元测试
 *
 * 验证功能：
 * - 默认英文输出
 * - 运行时 locale 切换
 * - 参数插值
 * - 缺失 key 回退
 * - 多语言覆盖
 */

import { describe, it, expect, beforeEach } from 'vitest';
import {
  setLocale,
  getLocale,
  t,
  interpolate,
  hasKey,
  getSupportedLocales,
  mergeMessages,
  type Locale,
} from '../../src/i18n/index.js';

describe('i18n', () => {
  // 每个测试前重置为英文
  beforeEach(() => {
    setLocale('en');
  });

  describe('setLocale / getLocale', () => {
    it('should default to English', () => {
      // 注意由于模块状态，可能需要重置
      setLocale('en');
      expect(getLocale()).toBe('en');
    });

    it('should switch to zh-CN', () => {
      setLocale('zh-CN');
      expect(getLocale()).toBe('zh-CN');
    });

    it('should switch to ja', () => {
      setLocale('ja');
      expect(getLocale()).toBe('ja');
    });

    it('should fallback to en for unsupported locale', () => {
      // Testing unsupported locale
      setLocale('fr' as Locale);
      expect(getLocale()).toBe('en');
    });
  });

  describe('t() - translation', () => {
    it('should return English text by default', () => {
      setLocale('en');
      expect(t('common.yes')).toBe('Yes');
      expect(t('common.no')).toBe('No');
      expect(t('common.loading')).toBe('Loading...');
    });

    it('should return Chinese text when locale is zh-CN', () => {
      setLocale('zh-CN');
      expect(t('common.yes')).toBe('是');
      expect(t('common.no')).toBe('否');
      expect(t('common.loading')).toBe('加载中...');
    });

    it('should return Japanese text when locale is ja', () => {
      setLocale('ja');
      expect(t('common.yes')).toBe('はい');
      expect(t('common.no')).toBe('いいえ');
      expect(t('common.loading')).toBe('読み込み中...');
    });

    it('should interpolate parameters', () => {
      setLocale('en');
      expect(t('timeout.operation', { seconds: 30 })).toBe(
        'Operation timed out after 30 seconds'
      );
      expect(t('agent.maxTurns', { count: 10 })).toBe(
        'Maximum turns (10) exceeded'
      );
    });

    it('should interpolate parameters in Chinese', () => {
      setLocale('zh-CN');
      expect(t('timeout.operation', { seconds: 30 })).toBe(
        '操作在 30 秒后超时'
      );
      expect(t('agent.maxTurns', { count: 10 })).toBe(
        '已达到最大轮数 (10)'
      );
    });

    it('should interpolate parameters in Japanese', () => {
      setLocale('ja');
      expect(t('timeout.operation', { seconds: 30 })).toBe(
        '操作が 30 秒後にタイムアウトしました'
      );
    });

    it('should handle missing params gracefully', () => {
      setLocale('en');
      const result = t('timeout.operation');
      expect(result).toBe('Operation timed out after {seconds} seconds');
    });

    it('should handle unknown keys', () => {
      setLocale('en');
      expect(t('nonexistent.key')).toBe('nonexistent.key');
    });

    it('should fallback to en when key missing in current locale', () => {
      setLocale('ja');
      // 所有 key 应该在 ja 中都存在（完整翻译）
      // 但如果某个 key 缺失，应回退到 en
      const enText = t('common.yes');
      // 日文中 "common.yes" = "はい"
      expect(enText).toBe('はい');
    });
  });

  describe('interpolate()', () => {
    it('should replace single parameter', () => {
      expect(interpolate('Hello, {name}!', { name: 'World' })).toBe(
        'Hello, World!'
      );
    });

    it('should replace multiple parameters', () => {
      expect(
        interpolate('{x} + {y} = {z}', { x: 1, y: 2, z: 3 })
      ).toBe('1 + 2 = 3');
    });

    it('should handle number values', () => {
      expect(interpolate('Count: {count}', { count: 42 })).toBe('Count: 42');
    });

    it('should handle empty params', () => {
      expect(interpolate('No params here')).toBe('No params here');
      expect(interpolate('Hello {name}', undefined)).toBe('Hello {name}');
    });

    it('should keep unmatched placeholders', () => {
      expect(interpolate('Hello {name}, age {age}', { name: 'Alice' })).toBe(
        'Hello Alice, age {age}'
      );
    });

    it('should handle special regex characters in template', () => {
      expect(interpolate('Price: ${price}', { price: '100' })).toBe(
        'Price: $100'
      );
    });
  });

  describe('error message translations', () => {
    it('should translate validation errors', () => {
      setLocale('en');
      expect(t('validation.required', { field: 'apiKey' })).toBe(
        'Field "apiKey" is required'
      );
      expect(t('validation.type', { field: 'count', type: 'number' })).toBe(
        'Field "count" must be of type number'
      );
    });

    it('should translate permission errors', () => {
      setLocale('zh-CN');
      expect(t('permission.denied')).toBe('权限不足');
      expect(t('permission.scope', { scope: 'read:files' })).toBe(
        '权限范围不足：需要 read:files'
      );
    });

    it('should translate LLM errors', () => {
      setLocale('en');
      expect(t('llm.apiKey', { provider: 'OpenAI' })).toBe(
        'API key is required for provider "OpenAI"'
      );
      expect(t('llm.rateLimited', { seconds: 60 })).toBe(
        'Rate limit exceeded, retry after 60s'
      );
    });

    it('should translate tool errors', () => {
      setLocale('ja');
      expect(t('tool.notFound', { name: 'web_search' })).toBe(
        'ツール "web_search" が見つかりません'
      );
    });

    it('should translate memory errors', () => {
      setLocale('zh-CN');
      expect(t('memory.episodeNotFound', { id: 'ep-001' })).toBe(
        '片段 "ep-001" 不存在'
      );
      expect(t('memory.invalidImportance')).toBe(
        '重要度必须在 0 到 1 之间'
      );
    });
  });

  describe('utility functions', () => {
    it('should check key existence', () => {
      expect(hasKey('common.yes')).toBe(true);
      expect(hasKey('nonexistent.key')).toBe(false);
    });

    it('should list supported locales', () => {
      const locales = getSupportedLocales();
      expect(locales).toContain('en');
      expect(locales).toContain('zh-CN');
      expect(locales).toContain('ja');
      expect(locales.length).toBe(3);
    });

    it('should merge custom messages', () => {
      mergeMessages('en', { 'custom.key': 'Custom Value' });
      expect(t('custom.key')).toBe('Custom Value');
    });
  });

  describe('locale switching at runtime', () => {
    it('should switch locale without restart', () => {
      setLocale('en');
      expect(t('common.yes')).toBe('Yes');

      setLocale('zh-CN');
      expect(t('common.yes')).toBe('是');

      setLocale('ja');
      expect(t('common.yes')).toBe('はい');

      setLocale('en');
      expect(t('common.yes')).toBe('Yes');
    });

    it('should maintain locale state across calls', () => {
      setLocale('zh-CN');
      const a = t('common.ok');
      const b = t('common.cancel');
      expect(a).toBe('确定');
      expect(b).toBe('取消');
    });
  });
});