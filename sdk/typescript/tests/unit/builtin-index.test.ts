import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import {
  JSONLoader,
  CSVLoader,
  HTMLLoader,
  MarkdownLoader,
  TextSplitter,
  PluginLoader,
  defaultToolkit,
} from '../../src/tools/builtin/index.js';

// ===== JSONLoader (builtin/index) tests =====
describe('JSONLoader (builtin)', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-json-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should load JSON from file', async () => {
    const filePath = path.join(tmpDir, 'test.json');
    fs.writeFileSync(filePath, JSON.stringify({ name: 'Alice', age: 30 }));
    const loader = new JSONLoader();
    const doc = await loader.load(filePath);
    expect(doc.metadata.format).toBe('json');
    expect(doc.metadata.source).toBe(filePath);
    expect(doc.content).toContain('Alice');
    expect(doc.content).toContain('30');
  });

  it('should load JSON from string', async () => {
    const loader = new JSONLoader();
    const doc = await loader.loadFromString('{"key": "value"}');
    expect(doc.metadata.source).toBe('inline');
    expect(doc.content).toContain('key');
    expect(doc.content).toContain('value');
  });

  it('should throw on invalid JSON', async () => {
    const loader = new JSONLoader();
    await expect(loader.loadFromString('{invalid}')).rejects.toThrow();
  });
});

// ===== CSVLoader (builtin/index) tests =====
describe('CSVLoader (builtin)', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-csv-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should load CSV from file', async () => {
    const filePath = path.join(tmpDir, 'test.csv');
    fs.writeFileSync(filePath, 'name,age\nAlice,30\nBob,25');
    const loader = new CSVLoader();
    const doc = await loader.load(filePath);
    expect(doc.metadata.format).toBe('csv');
    expect(doc.metadata.source).toBe(filePath);
    expect(doc.content).toContain('Alice');
  });

  it('should load CSV from string', async () => {
    const loader = new CSVLoader();
    const doc = await loader.loadFromString('a,b\n1,2');
    expect(doc.metadata.source).toBe('inline');
    expect(doc.content).toContain('a');
  });

  it('should handle quoted fields', async () => {
    const loader = new CSVLoader();
    const doc = await loader.loadFromString('name\n"Smith, John"');
    expect(doc.content).toContain('Smith, John');
  });

  it('should handle escaped quotes', async () => {
    const loader = new CSVLoader();
    const doc = await loader.loadFromString('text\n"He said ""hi"""');
    // builtin CSVLoader formats output as JSON objects
    expect(doc.content).toContain('He said');
    expect(doc.content).toContain('hi');
  });

  it('should handle empty CSV', async () => {
    const loader = new CSVLoader();
    const doc = await loader.loadFromString('');
    expect(doc.content).toBe('');
    expect(doc.metadata.size).toBe(0);
  });

  it('should include rows and columns in metadata', async () => {
    const loader = new CSVLoader();
    const doc = await loader.loadFromString('a,b,c\n1,2,3\n4,5,6');
    expect(doc.metadata.rows).toBe(2);
    expect(doc.metadata.columns).toBe(3);
  });
});

// ===== HTMLLoader (builtin/index) tests =====
describe('HTMLLoader (builtin)', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-html-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should load HTML from file', async () => {
    const filePath = path.join(tmpDir, 'test.html');
    fs.writeFileSync(filePath, '<html><body><p>Hello</p></body></html>');
    const loader = new HTMLLoader();
    const doc = await loader.load(filePath);
    expect(doc.metadata.format).toBe('html');
    expect(doc.content).toContain('Hello');
  });

  it('should load HTML from string', async () => {
    const loader = new HTMLLoader();
    const doc = await loader.loadFromString('<p>World</p>');
    expect(doc.metadata.source).toBe('inline');
    expect(doc.content).toContain('World');
  });

  it('should remove scripts and styles', async () => {
    const loader = new HTMLLoader();
    const doc = await loader.loadFromString('<script>alert(1)</script><style>x{}</style><p>Text</p>');
    expect(doc.content).not.toContain('alert');
    expect(doc.content).not.toContain('style');
    expect(doc.content).toContain('Text');
  });

  it('should decode HTML entities', async () => {
    const loader = new HTMLLoader();
    const doc = await loader.loadFromString('<p>&amp;&lt;&gt;</p>');
    expect(doc.content).toContain('&');
    expect(doc.content).toContain('<');
    expect(doc.content).toContain('>');
  });
});

// ===== MarkdownLoader (builtin/index) tests =====
describe('MarkdownLoader (builtin)', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-md-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should load markdown from file', async () => {
    const filePath = path.join(tmpDir, 'test.md');
    fs.writeFileSync(filePath, '# Title\n\nContent here');
    const loader = new MarkdownLoader();
    const doc = await loader.load(filePath);
    expect(doc.metadata.format).toBe('markdown');
    expect(doc.content).toContain('# Title');
    expect(doc.content).toContain('Content here');
  });
});

// ===== TextSplitter (builtin/index) tests =====
describe('TextSplitter (builtin)', () => {
  it('should return single chunk for short text', () => {
    const splitter = new TextSplitter();
    const chunks = splitter.split('short text');
    expect(chunks).toHaveLength(1);
  });

  it('should split long text', () => {
    const splitter = new TextSplitter({ chunkSize: 20, chunkOverlap: 5 });
    const text = 'a'.repeat(100);
    const chunks = splitter.split(text);
    expect(chunks.length).toBeGreaterThan(1);
  });

  it('should handle custom separator', () => {
    const splitter = new TextSplitter({ chunkSize: 10, separator: '|' });
    const text = 'aaaa|bbbb|cccc';
    const chunks = splitter.split(text);
    expect(chunks.length).toBeGreaterThan(0);
  });

  it('should split oversized chunks', () => {
    const splitter = new TextSplitter({ chunkSize: 10, chunkOverlap: 2 });
    const text = 'a'.repeat(100);
    const chunks = splitter.split(text);
    for (const chunk of chunks) {
      expect(chunk.length).toBeLessThanOrEqual(15); // chunkSize * 1.5
    }
  });

  it('should use defaults', () => {
    const splitter = new TextSplitter();
    const chunks = splitter.split('test');
    expect(chunks).toHaveLength(1);
  });
});

// ===== PluginLoader tests =====
describe('PluginLoader', () => {
  it('should load a plugin', async () => {
    const loader = new PluginLoader();
    const mockTool = { name: 'test', description: 'test', parameters: {}, execute: vi.fn() };
    await loader.load({
      name: 'test-plugin',
      version: '1.0.0',
      tools: [mockTool as any],
    });
    expect(loader.list()).toContain('test-plugin');
  });

  it('should call plugin init if provided', async () => {
    const loader = new PluginLoader();
    const initFn = vi.fn();
    await loader.load({
      name: 'test-plugin',
      version: '1.0.0',
      tools: [],
      init: initFn,
    });
    expect(initFn).toHaveBeenCalledWith({ config: {} });
  });

  it('should pass context to init', async () => {
    const loader = new PluginLoader();
    const initFn = vi.fn();
    await loader.load({
      name: 'test-plugin',
      version: '1.0.0',
      tools: [],
      init: initFn,
    }, { config: { key: 'value' } });
    expect(initFn).toHaveBeenCalledWith({ config: { key: 'value' } });
  });

  it('should get tools from all plugins', async () => {
    const loader = new PluginLoader();
    const tool1 = { name: 't1', description: 'd', parameters: {}, execute: vi.fn() };
    const tool2 = { name: 't2', description: 'd', parameters: {}, execute: vi.fn() };
    await loader.load({ name: 'p1', version: '1', tools: [tool1 as any] });
    await loader.load({ name: 'p2', version: '1', tools: [tool2 as any] });
    const tools = loader.getTools();
    expect(tools).toHaveLength(2);
  });

  it('should get plugin by name', async () => {
    const loader = new PluginLoader();
    await loader.load({ name: 'test', version: '1', tools: [] });
    expect(loader.getPlugin('test')).toBeDefined();
    expect(loader.getPlugin('missing')).toBeUndefined();
  });

  it('should list plugin names', async () => {
    const loader = new PluginLoader();
    await loader.load({ name: 'a', version: '1', tools: [] });
    await loader.load({ name: 'b', version: '1', tools: [] });
    expect(loader.list()).toEqual(['a', 'b']);
  });

  it('should unload plugins', async () => {
    const loader = new PluginLoader();
    await loader.load({ name: 'test', version: '1', tools: [] });
    expect(loader.unload('test')).toBe(true);
    expect(loader.list()).not.toContain('test');
  });

  it('should return false when unloading non-existent plugin', () => {
    const loader = new PluginLoader();
    expect(loader.unload('missing')).toBe(false);
  });
});

// ===== defaultToolkit tests =====
describe('defaultToolkit', () => {
  it('should create registry with FileSystemTool', () => {
    const registry = defaultToolkit({ enableFS: true, rootDir: '.' });
    expect(registry).toBeDefined();
    expect(registry.list().some(t => t.name === 'filesystem')).toBe(true);
  });

  it('should create registry with ShellTool', () => {
    const registry = defaultToolkit({ enableShell: true });
    expect(registry.list().some(t => t.name === 'shell')).toBe(true);
  });

  it('should create registry with WebTool', () => {
    const registry = defaultToolkit({ enableWeb: true });
    expect(registry.list().some(t => t.name === 'web')).toBe(true);
  });

  it('should create registry with DatabaseTool', () => {
    const registry = defaultToolkit({
      enableDatabase: true,
      dbConnection: { query: vi.fn() },
    });
    expect(registry.list().some(t => t.name === 'database')).toBe(true);
  });

  it('should not add DatabaseTool without dbConnection', () => {
    const registry = defaultToolkit({ enableDatabase: true });
    expect(registry.list().some(t => t.name === 'database')).toBe(false);
  });

  it('should create registry with CodeExecutionTool', () => {
    const registry = defaultToolkit({ enableCodeExecution: true });
    expect(registry.list().some(t => t.name === 'code_execution')).toBe(true);
  });

  it('should create registry with KnowledgeTool', () => {
    const registry = defaultToolkit({
      enableKnowledge: true,
      knowledgeSearchFn: vi.fn(),
    });
    expect(registry.list().some(t => t.name === 'knowledge_search')).toBe(true);
  });

  it('should not add KnowledgeTool without searchFn', () => {
    const registry = defaultToolkit({ enableKnowledge: true });
    expect(registry.list().some(t => t.name === 'knowledge_search')).toBe(false);
  });

  it('should create empty registry when no tools enabled', () => {
    const registry = defaultToolkit({});
    expect(registry.list()).toHaveLength(0);
  });

  it('should enable all tools together', () => {
    const registry = defaultToolkit({
      enableFS: true,
      enableShell: true,
      enableWeb: true,
      enableCodeExecution: true,
      enableDatabase: true,
      dbConnection: { query: vi.fn() },
      enableKnowledge: true,
      knowledgeSearchFn: vi.fn(),
    });
    expect(registry.list().length).toBeGreaterThanOrEqual(6);
  });
});
