// tests/unit/document-loader.test.ts
// 文档加载器单元测试，使用 t.TempDir 等价物：tmp 目录

import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { mkdtemp, writeFile, rm, mkdir } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import {
  TextLoader,
  MDLoader,
  JSONDocLoader,
  CodeLoader,
  DirectoryLoader,
} from '../../src/prompt/document-loader.js';

let dir: string;

beforeAll(async () => {
  dir = await mkdtemp(join(tmpdir(), 'ap-docloader-'));
  await writeFile(join(dir, 'a.txt'), 'hello world', 'utf-8');
  await writeFile(join(dir, 'b.md'), '# title\n\nbody', 'utf-8');
  await writeFile(join(dir, 'c.json'), JSON.stringify({ k: 1, v: [1, 2] }), 'utf-8');
  await writeFile(join(dir, 'main.ts'), 'export const x = 1;', 'utf-8');
  await mkdir(join(dir, 'sub'));
  await writeFile(join(dir, 'sub', 'd.go'), 'package main', 'utf-8');
  await writeFile(join(dir, 'ignored.bin'), 'binary', 'utf-8');
});

afterAll(async () => {
  await rm(dir, { recursive: true, force: true });
});

describe('TextLoader', () => {
  it('reads a text file and attaches metadata', async () => {
    const loader = new TextLoader();
    const docs = await loader.load(join(dir, 'a.txt'));
    expect(docs).toHaveLength(1);
    expect(docs[0].content).toBe('hello world');
    expect(docs[0].type).toBe('text');
    expect(docs[0].source).toContain('a.txt');
    expect(docs[0].metadata.encoding).toBe('utf-8');
    expect(docs[0].metadata.filename).toBe('a.txt');
  });
});

describe('MDLoader', () => {
  it('reads a markdown file', async () => {
    const loader = new MDLoader();
    const docs = await loader.load(join(dir, 'b.md'));
    expect(docs[0].content).toBe('# title\n\nbody');
    expect(docs[0].type).toBe('markdown');
  });
});

describe('JSONDocLoader', () => {
  it('parses and pretty-prints JSON content', async () => {
    const loader = new JSONDocLoader();
    const docs = await loader.load(join(dir, 'c.json'));
    const parsed = JSON.parse(docs[0].content);
    expect(parsed).toEqual({ k: 1, v: [1, 2] });
    expect(docs[0].type).toBe('json');
  });
});

describe('CodeLoader', () => {
  it('detects typescript language from extension', async () => {
    const loader = new CodeLoader();
    const docs = await loader.load(join(dir, 'main.ts'));
    expect(docs[0].type).toBe('code');
    expect(docs[0].metadata.language).toBe('typescript');
    expect(docs[0].content).toContain('export const x');
  });

  it('detects go language from extension', async () => {
    const loader = new CodeLoader();
    const docs = await loader.load(join(dir, 'sub', 'd.go'));
    expect(docs[0].metadata.language).toBe('go');
  });
});

describe('DirectoryLoader', () => {
  it('walks recursively and respects extension filter', async () => {
    const inner = new TextLoader();
    const dl = new DirectoryLoader(inner, ['.txt', '.md']);
    const docs = await dl.load(dir);
    // DirectoryLoader 内部使用 TextLoader 包装，因此 type 均为 'text'，
    // 但通过 metadata.filename 可区分原文件类型
    const filenames = docs.map((d) => d.metadata.filename).sort();
    expect(filenames).toEqual(['a.txt', 'b.md']);
    expect(docs).toHaveLength(2);
  });

  it('loads all files when no extension filter is supplied', async () => {
    const inner = new TextLoader();
    const dl = new DirectoryLoader(inner);
    const docs = await dl.load(dir);
    expect(docs.length).toBeGreaterThanOrEqual(5);
  });
});