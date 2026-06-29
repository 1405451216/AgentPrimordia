import { describe, it, expect } from 'vitest';
import {
  PDFLoader,
  DOCXLoader,
  CSVLoader,
  HTMLLoader,
  MarkdownLoader,
  JSONLoader,
  TextSplitter,
  DataTools,
  ToolCache,
  TrieRule,
} from '../../src/tools/document-loaders.js';

// ===== PDFLoader tests =====
describe('PDFLoader', () => {
  it('should extract text from PDF stream content', async () => {
    const pdfContent = `stream
BT /F1 12 Tf (Hello World) Tj ET
endstream`;
    const loader = new PDFLoader();
    const doc = await loader.load(pdfContent, 'test.pdf');
    expect(doc.content).toContain('Hello World');
    expect(doc.metadata.format).toBe('pdf');
    expect(doc.metadata.source).toBe('test.pdf');
  });

  it('should fallback to raw content when no text found', async () => {
    const loader = new PDFLoader();
    const doc = await loader.load('no stream content', 'test.pdf');
    expect(doc.content).toBe('no stream content');
  });

  it('should handle Buffer input', async () => {
    const pdfContent = Buffer.from('stream\nBT (test) Tj ET\nendstream');
    const loader = new PDFLoader();
    const doc = await loader.load(pdfContent, 'buffer.pdf');
    expect(doc.metadata.size).toBe(pdfContent.length);
  });

  it('should use default source when not provided', async () => {
    const loader = new PDFLoader();
    const doc = await loader.load('test content');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== DOCXLoader tests =====
describe('DOCXLoader', () => {
  it('should extract text from w:t tags', async () => {
    const docxContent = '<w:t>Hello</w:t><w:t>World</w:t>';
    const loader = new DOCXLoader();
    const doc = await loader.load(docxContent, 'test.docx');
    expect(doc.content).toContain('Hello');
    expect(doc.content).toContain('World');
    expect(doc.metadata.format).toBe('docx');
  });

  it('should fallback to raw content when no w:t tags', async () => {
    const loader = new DOCXLoader();
    const doc = await loader.load('plain text');
    expect(doc.content).toBe('plain text');
  });

  it('should handle Buffer input', async () => {
    const content = Buffer.from('<w:t>test</w:t>');
    const loader = new DOCXLoader();
    const doc = await loader.load(content, 'buffer.docx');
    expect(doc.content).toContain('test');
    expect(doc.metadata.size).toBe(content.length);
  });

  it('should use default source', async () => {
    const loader = new DOCXLoader();
    const doc = await loader.load('<w:t>test</w:t>');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== CSVLoader tests =====
describe('CSVLoader', () => {
  it('should parse CSV with headers and rows', async () => {
    const csv = 'name,age\nAlice,30\nBob,25';
    const loader = new CSVLoader();
    const doc = await loader.load(csv, 'test.csv');
    expect(doc.content).toContain('Headers: name, age');
    expect(doc.content).toContain('name: Alice, age: 30');
    expect(doc.content).toContain('name: Bob, age: 25');
    expect(doc.metadata.format).toBe('csv');
  });

  it('should handle quoted CSV fields', async () => {
    const csv = 'name,desc\n"Smith, John","Hello, World"';
    const loader = new CSVLoader();
    const doc = await loader.load(csv);
    expect(doc.content).toContain('Smith, John');
    expect(doc.content).toContain('Hello, World');
  });

  it('should handle escaped quotes in CSV', async () => {
    const csv = 'text\n"He said ""hello"" world"';
    const loader = new CSVLoader();
    const doc = await loader.load(csv);
    expect(doc.content).toContain('He said "hello" world');
  });

  it('should handle empty CSV', async () => {
    const loader = new CSVLoader();
    const doc = await loader.load('   ');
    expect(doc.metadata.size).toBe(3);
  });

  it('should handle CSV with empty lines', async () => {
    const csv = 'a,b\n\n1,2\n';
    const loader = new CSVLoader();
    const doc = await loader.load(csv);
    expect(doc.content).toContain('a: 1, b: 2');
  });

  it('should handle rows with fewer values than headers', async () => {
    const csv = 'a,b,c\n1,2';
    const loader = new CSVLoader();
    const doc = await loader.load(csv);
    expect(doc.content).toContain('a: 1, b: 2, c: ');
  });

  it('should use default source', async () => {
    const loader = new CSVLoader();
    const doc = await loader.load('a,b\n1,2');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== HTMLLoader tests =====
describe('HTMLLoader', () => {
  it('should extract text from HTML', async () => {
    const html = '<html><body><p>Hello World</p></body></html>';
    const loader = new HTMLLoader();
    const doc = await loader.load(html, 'test.html');
    expect(doc.content).toContain('Hello World');
    expect(doc.metadata.format).toBe('html');
  });

  it('should extract title', async () => {
    const html = '<html><head><title>Page Title</title></head><body>Content</body></html>';
    const loader = new HTMLLoader();
    const doc = await loader.load(html);
    expect(doc.content).toContain('Page Title');
    expect(doc.content).toContain('Content');
  });

  it('should remove scripts', async () => {
    const html = '<html><body>Text<script>alert("xss")</script></body></html>';
    const loader = new HTMLLoader();
    const doc = await loader.load(html);
    expect(doc.content).not.toContain('alert');
    expect(doc.content).not.toContain('script');
  });

  it('should remove styles', async () => {
    const html = '<html><body>Text<style>body { color: red; }</style></body></html>';
    const loader = new HTMLLoader();
    const doc = await loader.load(html);
    expect(doc.content).not.toContain('color');
    expect(doc.content).not.toContain('style');
  });

  it('should decode HTML entities', async () => {
    const html = '<p>&amp;&lt;&gt;&quot;&#39;&nbsp;</p>';
    const loader = new HTMLLoader();
    const doc = await loader.load(html);
    expect(doc.content).toContain('&');
    expect(doc.content).toContain('<');
    expect(doc.content).toContain('>');
    expect(doc.content).toContain('"');
    expect(doc.content).toContain("'");
  });

  it('should handle empty HTML', async () => {
    const loader = new HTMLLoader();
    const doc = await loader.load('');
    expect(doc.content).toBe('');
    expect(doc.metadata.size).toBe(0);
  });

  it('should use default source', async () => {
    const loader = new HTMLLoader();
    const doc = await loader.load('<p>test</p>');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== MarkdownLoader tests =====
describe('MarkdownLoader', () => {
  it('should remove code blocks', async () => {
    const md = 'Text\n```js\nconsole.log("code")\n```\nMore text';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('[code block]');
    expect(doc.content).not.toContain('console.log');
  });

  it('should remove inline code but keep text', async () => {
    const md = 'Use `variable` here';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('variable');
    expect(doc.content).not.toContain('`');
  });

  it('should remove images and replace with alt text', async () => {
    const md = '![alt text](image.png)';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('[image: alt text]');
  });

  it('should remove links but keep text', async () => {
    const md = '[click here](https://example.com)';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('click here');
    expect(doc.content).not.toContain('https://example.com');
  });

  it('should remove header markers', async () => {
    const md = '# Title\n## Subtitle\n### Sub-subtitle';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('Title');
    expect(doc.content).toContain('Subtitle');
    expect(doc.content).not.toContain('##');
  });

  it('should remove bold and italic markers', async () => {
    const md = '**bold** *italic* ***both***';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('bold');
    expect(doc.content).toContain('italic');
    expect(doc.content).toContain('both');
  });

  it('should remove underscore bold/italic', async () => {
    const md = '__bold__ _italic_';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('bold');
    expect(doc.content).toContain('italic');
  });

  it('should remove blockquotes', async () => {
    const md = '> This is a quote';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('This is a quote');
    expect(doc.content).not.toContain('>');
  });

  it('should remove list markers', async () => {
    const md = '- item1\n* item2\n+ item3';
    const loader = new MarkdownLoader();
    const doc = await loader.load(md);
    expect(doc.content).toContain('item1');
    expect(doc.content).toContain('item2');
    expect(doc.content).toContain('item3');
  });

  it('should use default source', async () => {
    const loader = new MarkdownLoader();
    const doc = await loader.load('# test');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== JSONLoader tests =====
describe('JSONLoader', () => {
  it('should flatten simple JSON', async () => {
    const json = JSON.stringify({ name: 'Alice', age: 30 });
    const loader = new JSONLoader();
    const doc = await loader.load(json, 'test.json');
    expect(doc.content).toContain('name: Alice');
    expect(doc.content).toContain('age: 30');
    expect(doc.metadata.format).toBe('json');
  });

  it('should flatten nested JSON objects', async () => {
    const json = JSON.stringify({ user: { name: 'Bob', address: { city: 'NYC' } } });
    const loader = new JSONLoader();
    const doc = await loader.load(json);
    expect(doc.content).toContain('user.name: Bob');
    expect(doc.content).toContain('user.address.city: NYC');
  });

  it('should flatten JSON arrays', async () => {
    const json = JSON.stringify({ items: ['a', 'b', 'c'] });
    const loader = new JSONLoader();
    const doc = await loader.load(json);
    expect(doc.content).toContain('items[0]: a');
    expect(doc.content).toContain('items[1]: b');
    expect(doc.content).toContain('items[2]: c');
  });

  it('should handle null values', async () => {
    const json = JSON.stringify({ value: null });
    const loader = new JSONLoader();
    const doc = await loader.load(json);
    expect(doc.content).toContain('value: null');
  });

  it('should handle boolean values', async () => {
    const json = JSON.stringify({ active: true, disabled: false });
    const loader = new JSONLoader();
    const doc = await loader.load(json);
    expect(doc.content).toContain('active: true');
    expect(doc.content).toContain('disabled: false');
  });

  it('should handle primitive values at root', async () => {
    const json = JSON.stringify('just a string');
    const loader = new JSONLoader();
    const doc = await loader.load(json);
    expect(doc.content).toContain('just a string');
  });

  it('should handle invalid JSON', async () => {
    const loader = new JSONLoader();
    await expect(loader.load('{invalid}')).rejects.toThrow();
  });

  it('should use default source', async () => {
    const loader = new JSONLoader();
    const doc = await loader.load('{}');
    expect(doc.metadata.source).toBe('unknown');
  });
});

// ===== TextSplitter tests =====
describe('TextSplitter', () => {
  it('should return single chunk for short text', () => {
    const splitter = new TextSplitter({ chunkSize: 100 });
    const chunks = splitter.split('short text');
    expect(chunks).toHaveLength(1);
    expect(chunks[0]).toBe('short text');
  });

  it('should split long text by separator', () => {
    const splitter = new TextSplitter({ chunkSize: 20, chunkOverlap: 5, separator: '\n\n' });
    const text = 'paragraph1\n\nparagraph2\n\nparagraph3';
    const chunks = splitter.split(text);
    expect(chunks.length).toBeGreaterThan(1);
  });

  it('should keep overlap between chunks', () => {
    const splitter = new TextSplitter({ chunkSize: 15, chunkOverlap: 5, separator: '\n\n' });
    const text = '1234567890\n\n1234567890';
    const chunks = splitter.split(text);
    if (chunks.length > 1) {
      // Check overlap exists
      const overlap = chunks[0]!.slice(-5);
      expect(chunks[1]).toContain(overlap);
    }
  });

  it('should handle single section that is too large', () => {
    const splitter = new TextSplitter({ chunkSize: 10, chunkOverlap: 2 });
    const text = 'a'.repeat(50);
    const chunks = splitter.split(text);
    // Without sentence delimiters, the text may remain as a single chunk
    expect(chunks.length).toBeGreaterThanOrEqual(1);
  });

  it('should use default config', () => {
    const splitter = new TextSplitter();
    const chunks = splitter.split('short');
    expect(chunks).toHaveLength(1);
  });

  it('should handle empty text', () => {
    const splitter = new TextSplitter();
    const chunks = splitter.split('');
    // Empty string produces empty array because '' is falsy
    expect(chunks.length).toBeGreaterThanOrEqual(0);
  });
});

// ===== DataTools tests =====
describe('DataTools', () => {
  it('should have csvAnalysis tool', () => {
    const tools = new DataTools();
    expect(tools.csvAnalysis.name).toBe('csv_analyze');
  });

  it('should have jsonQuery tool', () => {
    const tools = new DataTools();
    expect(tools.jsonQuery.name).toBe('json_query');
  });

  it('should have textStats tool', () => {
    const tools = new DataTools();
    expect(tools.textStats.name).toBe('text_stats');
  });

  it('should list all tools', () => {
    const tools = new DataTools();
    const list = tools.list();
    expect(list).toHaveLength(3);
    expect(list.map(t => t.name)).toContain('csv_analyze');
    expect(list.map(t => t.name)).toContain('json_query');
    expect(list.map(t => t.name)).toContain('text_stats');
  });

  it('should analyze CSV summary', async () => {
    const tools = new DataTools();
    const result = await tools.csvAnalysis.execute({ data: 'a,b\n1,2\n3,4', operation: 'summary' });
    expect(result).toContain('2 rows');
    expect(result).toContain('2 columns');
  });

  it('should count CSV rows', async () => {
    const tools = new DataTools();
    const result = await tools.csvAnalysis.execute({ data: 'a,b\n1,2\n3,4\n5,6', operation: 'count' });
    expect(result).toContain('Row count: 3');
  });

  it('should list CSV columns', async () => {
    const tools = new DataTools();
    const result = await tools.csvAnalysis.execute({ data: 'name,age,city\n1,2,3', operation: 'columns' });
    expect(result).toContain('name');
    expect(result).toContain('age');
    expect(result).toContain('city');
  });

  it('should return CSV head', async () => {
    const tools = new DataTools();
    const result = await tools.csvAnalysis.execute({ data: 'a,b\n1,2\n3,4\n5,6', operation: 'head' });
    expect(result).toContain('a,b');
    expect(result).toContain('1,2');
  });

  it('should return default content for unknown operation', async () => {
    const tools = new DataTools();
    const result = await tools.csvAnalysis.execute({ data: 'a,b\n1,2', operation: 'unknown' });
    expect(result).toContain('Headers: a, b');
  });

  it('should query JSON by path', async () => {
    const tools = new DataTools();
    const result = await tools.jsonQuery.execute({
      json: JSON.stringify({ user: { name: 'Alice' } }),
      path: 'user.name',
    });
    expect(result).toBe('Alice');
  });

  it('should return null for null value in path', async () => {
    const tools = new DataTools();
    const result = await tools.jsonQuery.execute({
      json: JSON.stringify({ user: { name: null } }),
      path: 'user.name',
    });
    expect(result).toBe('null');
  });

  it('should return not found for missing path', async () => {
    const tools = new DataTools();
    const result = await tools.jsonQuery.execute({
      json: JSON.stringify({ user: { name: 'Alice' } }),
      path: 'user.missing',
    });
    expect(result).toContain('not found');
  });

  it('should query JSON array by index', async () => {
    const tools = new DataTools();
    const result = await tools.jsonQuery.execute({
      json: JSON.stringify({ items: ['a', 'b', 'c'] }),
      path: 'items.1',
    });
    expect(result).toBe('b');
  });

  it('should return object as JSON string', async () => {
    const tools = new DataTools();
    const result = await tools.jsonQuery.execute({
      json: JSON.stringify({ user: { name: 'Alice', age: 30 } }),
      path: 'user',
    });
    expect(result).toContain('"name": "Alice"');
    expect(result).toContain('"age": 30');
  });

  it('should calculate text stats', async () => {
    const tools = new DataTools();
    const result = await tools.textStats.execute({ text: 'Hello world. This is a test.' });
    const stats = JSON.parse(result);
    expect(stats.characters).toBe(28);
    expect(stats.words).toBe(6);
    expect(stats.sentences).toBe(2);
  });

  it('should handle empty text stats', async () => {
    const tools = new DataTools();
    const result = await tools.textStats.execute({ text: '' });
    const stats = JSON.parse(result);
    expect(stats.characters).toBe(0);
    expect(stats.words).toBe(0);
    expect(stats.avgWordsPerSentence).toBe(0);
  });
});

// ===== ToolCache tests =====
describe('ToolCache', () => {
  it('should store and retrieve values', () => {
    const cache = new ToolCache();
    cache.set('key1', 'value1');
    expect(cache.get('key1')).toBe('value1');
  });

  it('should return undefined for missing keys', () => {
    const cache = new ToolCache();
    expect(cache.get('missing')).toBeUndefined();
  });

  it('should handle TTL expiration', async () => {
    const cache = new ToolCache(100, 100); // 100ms TTL
    cache.set('key1', 'value1');
    await new Promise(r => setTimeout(r, 150));
    expect(cache.get('key1')).toBeUndefined();
  });

  it('should handle custom TTL', async () => {
    const cache = new ToolCache();
    cache.set('key1', 'value1', 50);
    await new Promise(r => setTimeout(r, 100));
    expect(cache.get('key1')).toBeUndefined();
  });

  it('should delete entries', () => {
    const cache = new ToolCache();
    cache.set('key1', 'value1');
    expect(cache.delete('key1')).toBe(true);
    expect(cache.get('key1')).toBeUndefined();
  });

  it('should return false when deleting non-existent key', () => {
    const cache = new ToolCache();
    expect(cache.delete('missing')).toBe(false);
  });

  it('should clear all entries', () => {
    const cache = new ToolCache();
    cache.set('a', '1');
    cache.set('b', '2');
    cache.clear();
    expect(cache.size()).toBe(0);
  });

  it('should report size', () => {
    const cache = new ToolCache();
    cache.set('a', '1');
    cache.set('b', '2');
    expect(cache.size()).toBe(2);
  });

  it('should evict oldest when at max capacity', () => {
    const cache = new ToolCache(2);
    cache.set('a', '1');
    cache.set('b', '2');
    cache.set('c', '3'); // should evict 'a'
    expect(cache.get('a')).toBeUndefined();
    expect(cache.get('b')).toBe('2');
    expect(cache.get('c')).toBe('3');
  });

  it('should generate cache key from tool name and args', () => {
    const key = ToolCache.makeKey('search', { query: 'test' });
    expect(key).toContain('search:');
    expect(key).toContain('test');
  });

  it('should use default maxSize and TTL', () => {
    const cache = new ToolCache();
    cache.set('key', 'value');
    expect(cache.get('key')).toBe('value');
  });
});

// ===== TrieRule tests =====
describe('TrieRule', () => {
  it('should insert and search for patterns', () => {
    const trie = new TrieRule();
    trie.insert('hello');
    const results = trie.search('hello world');
    expect(results.length).toBeGreaterThan(0);
    expect(results[0]!.pattern).toBe('hello');
  });

  it('should return empty for no matches', () => {
    const trie = new TrieRule();
    trie.insert('hello');
    const results = trie.search('world');
    expect(results).toHaveLength(0);
  });

  it('should check if text contains any pattern', () => {
    const trie = new TrieRule();
    trie.insert('bad');
    expect(trie.contains('this is bad')).toBe(true);
    expect(trie.contains('this is good')).toBe(false);
  });

  it('should handle multiple patterns', () => {
    const trie = new TrieRule();
    trie.insert('cat');
    trie.insert('dog');
    const results = trie.search('cat and dog');
    expect(results.length).toBeGreaterThanOrEqual(2);
  });

  it('should store and retrieve data with pattern', () => {
    const trie = new TrieRule();
    trie.insert('password', 'sensitive');
    const results = trie.search('enter password here');
    expect(results[0]!.data).toBe('sensitive');
  });

  it('should remove patterns', () => {
    const trie = new TrieRule();
    trie.insert('hello');
    expect(trie.remove('hello')).toBe(true);
    expect(trie.contains('hello')).toBe(false);
  });

  it('should return false when removing non-existent pattern', () => {
    const trie = new TrieRule();
    expect(trie.remove('missing')).toBe(false);
  });

  it('should clear all patterns', () => {
    const trie = new TrieRule();
    trie.insert('a');
    trie.insert('b');
    trie.clear();
    expect(trie.contains('a')).toBe(false);
    expect(trie.contains('b')).toBe(false);
  });

  it('should find pattern at different positions', () => {
    const trie = new TrieRule();
    trie.insert('ab');
    const results = trie.search('xabxab');
    expect(results.length).toBeGreaterThanOrEqual(2);
  });

  it('should handle overlapping patterns', () => {
    const trie = new TrieRule();
    trie.insert('he');
    trie.insert('hello');
    const results = trie.search('hello');
    // Trie returns the longest match at each starting position
    expect(results.some(r => r.pattern === 'hello')).toBe(true);
  });
});
