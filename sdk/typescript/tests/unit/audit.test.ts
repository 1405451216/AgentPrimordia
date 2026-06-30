import { describe, it, expect, beforeEach } from 'vitest';
import { AuditLogger, InMemoryAuditOutput } from '../../src/audit/logger.js';

describe('AuditLogger', () => {
  let output: InMemoryAuditOutput;
  let logger: AuditLogger;

  beforeEach(() => {
    output = new InMemoryAuditOutput();
    logger = new AuditLogger({ output });
  });

  describe('log', () => {
    it('logs an event with auto timestamp', async () => {
      await logger.log({ actor: 'user1', action: 'create', resource: 'task/1' });
      expect(output.count()).toBe(1);
    });

    it('logs an event with custom timestamp', async () => {
      await logger.log({
        actor: 'user1',
        action: 'create',
        resource: 'task/1',
        timestamp: '2024-01-01T00:00:00Z',
      });
      const events = await output.query({});
      expect(events[0]!.timestamp).toBe('2024-01-01T00:00:00Z');
    });

    it('defaults result to success', async () => {
      await logger.log({ actor: 'user1', action: 'create' });
      const events = await output.query({});
      expect(events[0]!.result).toBe('success');
    });

    it('stores custom result', async () => {
      await logger.log({ actor: 'user1', action: 'create', result: 'failure' });
      const events = await output.query({});
      expect(events[0]!.result).toBe('failure');
    });

    it('defaults resource to empty string', async () => {
      await logger.log({ actor: 'user1', action: 'create' });
      const events = await output.query({});
      expect(events[0]!.resource).toBe('');
    });
  });

  describe('query', () => {
    beforeEach(async () => {
      await logger.log({ actor: 'alice', action: 'read', resource: 'doc/1' });
      await logger.log({ actor: 'bob', action: 'write', resource: 'doc/2' });
      await logger.log({ actor: 'alice', action: 'write', resource: 'doc/1' });
    });

    it('queries all events', async () => {
      const events = await logger.query({});
      expect(events.length).toBe(3);
    });

    it('filters by actor', async () => {
      const events = await logger.query({ actor: 'alice' });
      expect(events.length).toBe(2);
      expect(events.every((e) => e.actor === 'alice')).toBe(true);
    });

    it('filters by action', async () => {
      const events = await logger.query({ action: 'write' });
      expect(events.length).toBe(2);
    });

    it('filters by resource', async () => {
      const events = await logger.query({ resource: 'doc/1' });
      expect(events.length).toBe(2);
    });

    it('applies limit', async () => {
      const events = await logger.query({ limit: 1 });
      expect(events.length).toBe(1);
    });
  });

  describe('generateReport', () => {
    beforeEach(async () => {
      await logger.log({ actor: 'alice', action: 'read', resource: 'doc/1', timestamp: '2024-01-01T00:00:00Z' });
      await logger.log({ actor: 'alice', action: 'write', resource: 'doc/1', timestamp: '2024-01-02T00:00:00Z' });
      await logger.log({ actor: 'bob', action: 'read', resource: 'doc/2', timestamp: '2024-01-03T00:00:00Z' });
    });

    it('generates a compliance report', async () => {
      const report = await logger.generateReport('2024-01-01T00:00:00Z', '2024-12-31T00:00:00Z');
      expect(report.totalEvents).toBe(3);
      expect(report.actorStats.size).toBe(2);
      expect(report.actionStats.size).toBe(2);
    });

    it('counts actor actions correctly', async () => {
      const report = await logger.generateReport('2024-01-01T00:00:00Z', '2024-12-31T00:00:00Z');
      const alice = report.actorStats.get('alice');
      expect(alice?.totalActions).toBe(2);
      expect(alice?.actions.get('read')).toBe(1);
      expect(alice?.actions.get('write')).toBe(1);
    });

    it('counts action stats correctly', async () => {
      const report = await logger.generateReport('2024-01-01T00:00:00Z', '2024-12-31T00:00:00Z');
      expect(report.actionStats.get('read')).toBe(2);
      expect(report.actionStats.get('write')).toBe(1);
    });
  });

  describe('exportReportJSON', () => {
    it('exports a valid JSON string', async () => {
      await logger.log({ actor: 'alice', action: 'read', resource: 'doc/1' });
      const report = await logger.generateReport('2020-01-01T00:00:00Z', '2030-12-31T00:00:00Z');
      const json = logger.exportReportJSON(report);
      const parsed = JSON.parse(json);
      expect(parsed.totalEvents).toBe(1);
      expect(parsed.actorStats.alice.totalActions).toBe(1);
    });
  });

  describe('constructor validation', () => {
    it('throws if output is null', () => {
      expect(() => new AuditLogger({ output: null as unknown as InMemoryAuditOutput }))
        .toThrow();
    });
  });

  describe('InMemoryAuditOutput', () => {
    it('clears events', async () => {
      await output.write({ timestamp: '2024-01-01T00:00:00Z', actor: 'a', action: 'b', resource: '', result: 'success' });
      expect(output.count()).toBe(1);
      output.clear();
      expect(output.count()).toBe(0);
    });
  });
});
