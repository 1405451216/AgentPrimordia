import { describe, it, expect } from 'vitest';
import { PromptEngine, FewShotPrompt, PromptParser, PromptRegistry } from '../../src/prompt/engine.js';
import { KeywordSelector, FewShotTemplate } from '../../src/prompt/few-shot.js';
import { JSONParser, MarkdownParser, RegexParser } from '../../src/prompt/parser.js';
import { TemplateRegistry } from '../../src/prompt/registry.js';
import { PromptTemplate, defaultSystemPrompt, codeAssistantTemplate, ragContextTemplate, formatRAGDocuments } from '../../src/agent/prompt-template.js';
import type { Example } from '../../src/prompt/few-shot.js';

// ===== PromptEngine tests =====
describe('PromptEngine', () => {
  it('should register and get template', () => {
    const engine = new PromptEngine();
    engine.register('greet', 'Hello {{name}}!');
    const tpl = engine.get('greet');
    expect(tpl).toBeDefined();
    expect(tpl!.name).toBe('greet');
  });

  it('should return undefined for unknown template', () => {
    const engine = new PromptEngine();
    expect(engine.get('unknown')).toBeUndefined();
  });

  it('should render template with variables', () => {
    const engine = new PromptEngine();
    engine.register('greet', 'Hello {{name}}!', 'You are {{role}}');
    const messages = engine.render('greet', { name: 'Alice', role: 'assistant' });
    expect(messages).toHaveLength(2);
    expect(messages[0]).toEqual({ role: 'system', content: 'You are assistant' });
    expect(messages[1]).toEqual({ role: 'user', content: 'Hello Alice!' });
  });

  it('should render without system prompt', () => {
    const engine = new PromptEngine();
    engine.register('simple', 'Hi {{name}}');
    const messages = engine.render('simple', { name: 'Bob' });
    expect(messages).toHaveLength(1);
    expect(messages[0]).toEqual({ role: 'user', content: 'Hi Bob' });
  });

  it('should throw for unknown template on render', () => {
    const engine = new PromptEngine();
    expect(() => engine.render('unknown', {})).toThrow('not found');
  });

  it('should list templates', () => {
    const engine = new PromptEngine();
    engine.register('a', 'A');
    engine.register('b', 'B');
    expect(engine.list()).toEqual(['a', 'b']);
  });

  it('should check if template exists', () => {
    const engine = new PromptEngine();
    engine.register('exists', 'test');
    expect(engine.has('exists')).toBe(true);
    expect(engine.has('missing')).toBe(false);
  });

  it('should handle multiple variables', () => {
    const engine = new PromptEngine();
    engine.register('multi', '{{greeting}} {{name}}, you are {{role}}');
    const messages = engine.render('multi', { greeting: 'Hi', name: 'Alice', role: 'admin' });
    expect(messages[0].content).toBe('Hi Alice, you are admin');
  });

  it('should leave unmatched variables as-is', () => {
    const engine = new PromptEngine();
    engine.register('vars', 'Hello {{name}}, {{missing}}');
    const messages = engine.render('vars', { name: 'Alice' });
    expect(messages[0].content).toBe('Hello Alice, {{missing}}');
  });
});

// ===== FewShotPrompt tests =====
describe('FewShotPrompt', () => {
  it('should build QA format', () => {
    const fs = new FewShotPrompt('qa');
    fs.addExample('What is 2+2?', '4');
    const messages = fs.build('What is 3+3?');
    expect(messages).toHaveLength(3);
    expect(messages[0]).toEqual({ role: 'user', content: 'What is 2+2?' });
    expect(messages[1]).toEqual({ role: 'assistant', content: '4' });
    expect(messages[2]).toEqual({ role: 'user', content: 'What is 3+3?' });
  });

  it('should build QA format with system', () => {
    const fs = new FewShotPrompt('qa');
    fs.addExample('Hi', 'Hello');
    const messages = fs.build('How are you?', 'Be helpful');
    expect(messages[0]).toEqual({ role: 'system', content: 'Be helpful' });
  });

  it('should build chat format', () => {
    const fs = new FewShotPrompt('chat');
    fs.addExample('Hello', 'Hi there');
    const messages = fs.build('How are you?');
    expect(messages).toHaveLength(1);
    expect(messages[0].role).toBe('system');
    expect(messages[0].content).toContain('Hello');
    expect(messages[0].content).toContain('Hi there');
    expect(messages[0].content).toContain('How are you?');
  });

  it('should build json format', () => {
    const fs = new FewShotPrompt('json');
    fs.addExample('input1', 'output1');
    const messages = fs.build('query1');
    expect(messages).toHaveLength(1);
    expect(messages[0].content).toContain('input1');
    expect(messages[0].content).toContain('output1');
    expect(messages[0].content).toContain('query1');
  });

  it('should use default qa format', () => {
    const fs = new FewShotPrompt();
    fs.addExample('Q', 'A');
    const messages = fs.build('query');
    expect(messages[0].role).toBe('user');
    expect(messages[0].content).toBe('Q');
  });

  it('should handle multiple examples in qa', () => {
    const fs = new FewShotPrompt('qa');
    fs.addExample('Q1', 'A1');
    fs.addExample('Q2', 'A2');
    const messages = fs.build('Q3');
    expect(messages).toHaveLength(5);
  });
});

// ===== PromptParser tests =====
describe('PromptParser', () => {
  it('should parse single role', () => {
    const parser = new PromptParser();
    const result = parser.parse('@user: Hello world');
    expect(result).toHaveLength(1);
    expect(result[0].role).toBe('user');
    expect(result[0].content).toBe('Hello world');
  });

  it('should parse multiple roles', () => {
    const parser = new PromptParser();
    const result = parser.parse('@system: You are helpful\n@user: Hi');
    expect(result).toHaveLength(2);
    expect(result[0].role).toBe('system');
    expect(result[0].content).toBe('You are helpful');
    expect(result[1].role).toBe('user');
    expect(result[1].content).toBe('Hi');
  });

  it('should handle multi-line content', () => {
    const parser = new PromptParser();
    const result = parser.parse('@user: Line 1\nLine 2\nLine 3');
    expect(result[0].content).toBe('Line 1\nLine 2\nLine 3');
  });

  it('should extract variables', () => {
    const parser = new PromptParser();
    const result = parser.parse('@user: Hello {{name}}, your role is {{role}}');
    expect(result[0].variables).toEqual(['name', 'role']);
  });

  it('should handle no roles', () => {
    const parser = new PromptParser();
    const result = parser.parse('just text');
    expect(result).toHaveLength(0);
  });

  it('should handle empty role content', () => {
    const parser = new PromptParser();
    const result = parser.parse('@user: ');
    expect(result).toHaveLength(1);
    expect(result[0].content).toBe('');
  });

  it('extractVars should find all variables', () => {
    const parser = new PromptParser();
    const vars = parser.extractVars('Hello {{name}} {{age}} {{name}}');
    expect(vars).toEqual(['name', 'age']);
  });

  it('extractVars should return empty for no variables', () => {
    const parser = new PromptParser();
    const vars = parser.extractVars('no variables here');
    expect(vars).toEqual([]);
  });

  it('fillTemplate should replace variables', () => {
    const parser = new PromptParser();
    const result = parser.fillTemplate('Hello {{name}}!', { name: 'Alice' });
    expect(result).toBe('Hello Alice!');
  });

  it('fillTemplate should replace missing variables with empty', () => {
    const parser = new PromptParser();
    const result = parser.fillTemplate('Hello {{name}}!', {});
    expect(result).toBe('Hello !');
  });
});

// ===== PromptRegistry tests =====
describe('PromptRegistry', () => {
  it('should register and get template', () => {
    const reg = new PromptRegistry();
    reg.register('greet', 'Hello {{name}}', { description: 'Greeting', tags: ['social'] });
    expect(reg.get('greet')).toBe('Hello {{name}}');
  });

  it('should return undefined for unknown', () => {
    const reg = new PromptRegistry();
    expect(reg.get('unknown')).toBeUndefined();
  });

  it('should list templates', () => {
    const reg = new PromptRegistry();
    reg.register('a', 'A', { description: 'A template' });
    reg.register('b', 'B', { tags: ['test'] });
    const list = reg.list();
    expect(list).toHaveLength(2);
    expect(list[0].name).toBe('a');
    expect(list[0].description).toBe('A template');
    expect(list[1].tags).toEqual(['test']);
  });

  it('should search by name', () => {
    const reg = new PromptRegistry();
    reg.register('greeting', 'Hi');
    reg.register('farewell', 'Bye');
    const results = reg.search('greet');
    expect(results).toHaveLength(1);
    expect(results[0].name).toBe('greeting');
  });

  it('should search by description', () => {
    const reg = new PromptRegistry();
    reg.register('a', 'A', { description: 'a greeting template' });
    const results = reg.search('greeting');
    expect(results).toHaveLength(1);
  });

  it('should search by tag', () => {
    const reg = new PromptRegistry();
    reg.register('a', 'A', { tags: ['social'] });
    const results = reg.search('social');
    expect(results).toHaveLength(1);
  });

  it('should delete template', () => {
    const reg = new PromptRegistry();
    reg.register('a', 'A');
    expect(reg.delete('a')).toBe(true);
    expect(reg.delete('a')).toBe(false);
  });

  it('toMessages should parse and fill', () => {
    const reg = new PromptRegistry();
    reg.register('greet', '@system: You are {{role}}\n@user: Hello {{name}}');
    const messages = reg.toMessages('greet', { role: 'assistant', name: 'Alice' });
    expect(messages).toHaveLength(2);
    expect(messages[0]).toEqual({ role: 'system', content: 'You are assistant' });
    expect(messages[1]).toEqual({ role: 'user', content: 'Hello Alice' });
  });

  it('toMessages should throw for unknown', () => {
    const reg = new PromptRegistry();
    expect(() => reg.toMessages('unknown', {})).toThrow('not found');
  });
});

// ===== KeywordSelector tests =====
describe('KeywordSelector', () => {
  it('should select examples by keyword overlap', () => {
    const selector = new KeywordSelector();
    const examples: Example[] = [
      { input: 'how to cook rice', output: 'boil water' },
      { input: 'how to code python', output: 'use editor' },
      { input: 'best restaurants nearby', output: 'use maps' },
    ];
    const selected = selector.selectExamples('how to code javascript', examples);
    // Both "how to cook rice" and "how to code python" share "how", "to"
    // "how to code python" also shares "code"
    expect(selected.length).toBeGreaterThanOrEqual(1);
    expect(selected[0].input).toBe('how to code python'); // highest score
  });

  it('should filter out zero-score examples', () => {
    const selector = new KeywordSelector();
    const examples: Example[] = [
      { input: 'aaa bbb', output: 'x' },
      { input: 'ccc ddd', output: 'y' },
    ];
    const selected = selector.selectExamples('eee fff', examples);
    expect(selected).toHaveLength(0);
  });

  it('should sort by relevance', () => {
    const selector = new KeywordSelector();
    const examples: Example[] = [
      { input: 'foo bar', output: '1' },
      { input: 'foo bar baz', output: '2' },
    ];
    const selected = selector.selectExamples('foo bar baz', examples);
    expect(selected[0].input).toBe('foo bar baz');
  });
});

// ===== FewShotTemplate tests =====
describe('FewShotTemplate', () => {
  it('should add and count examples', () => {
    const tpl = new FewShotTemplate({
      baseTemplate: new PromptTemplate('Task: {{task}}'),
    });
    tpl.addExample({ input: 'q1', output: 'a1' });
    tpl.addExamples([{ input: 'q2', output: 'a2' }, { input: 'q3', output: 'a3' }]);
    expect(tpl.getExampleCount()).toBe(3);
  });

  it('should render with examples', () => {
    const tpl = new FewShotTemplate({
      baseTemplate: new PromptTemplate('Task: {{.task}}'),
      maxExamples: 5,
    });
    tpl.addExample({ input: 'hello world', output: 'hi there' });
    const result = tpl.render('hello world test', { task: 'greeting' });
    expect(result).toContain('Task: greeting');
    expect(result).toContain('hello world');
    expect(result).toContain('hi there');
    expect(result).toContain('hello world test');
  });

  it('should respect maxExamples', () => {
    const tpl = new FewShotTemplate({
      baseTemplate: new PromptTemplate('Task: {{.task}}'),
      maxExamples: 2,
    });
    for (let i = 0; i < 5; i++) {
      tpl.addExample({ input: `test ${i}`, output: `result ${i}` });
    }
    const result = tpl.render('test', { task: 'demo' });
    // Should only include 2 examples (maxExamples)
    const exampleCount = (result.match(/test \d/g) || []).length;
    expect(exampleCount).toBeLessThanOrEqual(3); // 2 examples + query
  });

  it('should render without examples when none match', () => {
    const tpl = new FewShotTemplate({
      baseTemplate: new PromptTemplate('Task: {{.task}}'),
    });
    tpl.addExample({ input: 'aaa', output: 'bbb' });
    const result = tpl.render('completely different', { task: 'test' });
    expect(result).toContain('Task: test');
    expect(result).not.toContain('aaa');
  });

  it('should use custom format', () => {
    const tpl = new FewShotTemplate({
      baseTemplate: new PromptTemplate('Task'),
      exampleFormat: 'IN: {{.Input}} OUT: {{.Output}}',
      prefix: '===',
      suffix: '---',
    });
    tpl.addExample({ input: 'q', output: 'a' });
    const result = tpl.render('q', {});
    expect(result).toContain('IN: q OUT: a');
    expect(result).toContain('===');
    expect(result).toContain('---');
  });
});

// ===== JSONParser tests =====
describe('JSONParser', () => {
  it('should parse valid JSON', () => {
    const parser = new JSONParser();
    const result = parser.parse('{"key": "value"}');
    expect(result).toEqual({ key: 'value' });
  });

  it('should parse JSON from markdown code block', () => {
    const parser = new JSONParser();
    const result = parser.parse('```json\n{"key": "value"}\n```');
    expect(result).toEqual({ key: 'value' });
  });

  it('should parse JSON from code block without lang', () => {
    const parser = new JSONParser();
    const result = parser.parse('```\n{"key": "value"}\n```');
    expect(result).toEqual({ key: 'value' });
  });

  it('should throw for invalid JSON', () => {
    const parser = new JSONParser();
    expect(() => parser.parse('not json')).toThrow('未找到');
  });

  it('should throw for non-object JSON in code block', () => {
    const parser = new JSONParser();
    // extractJSON can find arrays inside ```json blocks
    expect(() => parser.parse('```json\n[1, 2, 3]\n```')).toThrow('不是 JSON 对象');
  });

  it('should validate keys with schema', () => {
    const parser = new JSONParser({
      schema: { name: 'string', age: 'number' },
      keysOnly: true,
      allowExtra: false,
    });
    expect(() => parser.parse('{"name": "Alice", "extra": "bad"}')).toThrow('不允许的键');
  });

  it('should allow valid keys with schema', () => {
    const parser = new JSONParser({
      schema: { name: 'string' },
      keysOnly: true,
    });
    const result = parser.parse('{"name": "Alice"}');
    expect(result).toEqual({ name: 'Alice' });
  });

  it('should return format instructions with schema', () => {
    const parser = new JSONParser({ schema: { name: 'string' } });
    const instructions = parser.formatInstructions();
    expect(instructions).toContain('JSON Schema');
    expect(instructions).toContain('name');
  });

  it('should return format instructions without schema', () => {
    const parser = new JSONParser();
    const instructions = parser.formatInstructions();
    expect(instructions).toContain('JSON');
  });

  it('should return type', () => {
    const parser = new JSONParser();
    expect(parser.getType()).toBe('json');
  });

  it('should handle malformed JSON in code block', () => {
    const parser = new JSONParser();
    expect(() => parser.parse('```json\n{invalid}\n```')).toThrow();
  });
});

// ===== MarkdownParser tests =====
describe('MarkdownParser', () => {
  it('should parse code block with language', () => {
    const parser = new MarkdownParser('python');
    const result = parser.parse('```python\nprint("hello")\n```');
    expect(result).toBe('print("hello")');
  });

  it('should parse code block without language', () => {
    const parser = new MarkdownParser();
    const result = parser.parse('```json\n{"key": "val"}\n```');
    expect(result).toBe('{"key": "val"}');
  });

  it('should fallback to text when no code block', () => {
    const parser = new MarkdownParser('python');
    const result = parser.parse('just plain text');
    expect(result).toBe('just plain text');
  });

  it('should return text if no code block', () => {
    const parser = new MarkdownParser();
    const result = parser.parse('just plain text');
    expect(result).toBe('just plain text');
  });

  it('should return format instructions', () => {
    const parser = new MarkdownParser('yaml');
    const instructions = parser.formatInstructions();
    expect(instructions).toContain('yaml');
  });

  it('should return type', () => {
    const parser = new MarkdownParser();
    expect(parser.getType()).toBe('markdown');
  });
});

// ===== RegexParser tests =====
describe('RegexParser', () => {
  it('should parse with group names', () => {
    const parser = new RegexParser(/Name: (\w+) Age: (\d+)/, ['name', 'age']);
    const result = parser.parse('Name: Alice Age: 30');
    expect(result).toEqual({ name: 'Alice', age: '30' });
  });

  it('should parse without group names', () => {
    const parser = new RegexParser(/(\w+) (\d+)/);
    const result = parser.parse('Alice 30');
    expect(result).toEqual({ group1: 'Alice', group2: '30' });
  });

  it('should accept string pattern', () => {
    const parser = new RegexParser('(\\w+)', ['word']);
    const result = parser.parse('hello');
    expect(result).toEqual({ word: 'hello' });
  });

  it('should throw when no match', () => {
    const parser = new RegexParser(/(\d+)/, ['num']);
    expect(() => parser.parse('no numbers')).toThrow('不匹配');
  });

  it('should return format instructions', () => {
    const parser = new RegexParser(/test/);
    expect(parser.formatInstructions()).toContain('格式');
  });

  it('should return type', () => {
    const parser = new RegexParser(/test/);
    expect(parser.getType()).toBe('regex');
  });
});

// ===== TemplateRegistry tests =====
describe('TemplateRegistry', () => {
  it('should register and get template', () => {
    const reg = new TemplateRegistry();
    const tpl = new PromptTemplate('Hello {{.Name}}').withVar('Name', 'Default');
    reg.register('greet', tpl);
    const got = reg.get('greet');
    expect(got).not.toBeNull();
    expect(got!.render()).toContain('Hello');
  });

  it('should return null for unknown template', () => {
    const reg = new TemplateRegistry();
    expect(reg.get('unknown')).toBeNull();
  });

  it('should throw on duplicate registration', () => {
    const reg = new TemplateRegistry();
    reg.register('a', new PromptTemplate('A'));
    expect(() => reg.register('a', new PromptTemplate('B'))).toThrow('已存在');
  });

  it('should render template', () => {
    const reg = new TemplateRegistry();
    reg.register('greet', new PromptTemplate('Hello {{.Name}}'));
    const result = reg.render('greet', { Name: 'Alice' });
    expect(result).toBe('Hello Alice');
  });

  it('should throw on render unknown', () => {
    const reg = new TemplateRegistry();
    expect(() => reg.render('unknown', {})).toThrow('不存在');
  });

  it('should unregister template', () => {
    const reg = new TemplateRegistry();
    reg.register('a', new PromptTemplate('A'));
    expect(reg.unregister('a')).toBe(true);
    expect(reg.unregister('a')).toBe(false);
  });

  it('should list templates', () => {
    const reg = new TemplateRegistry();
    reg.register('a', new PromptTemplate('A'));
    reg.register('b', new PromptTemplate('B'));
    expect(reg.list()).toEqual(['a', 'b']);
  });

  it('should check has', () => {
    const reg = new TemplateRegistry();
    reg.register('a', new PromptTemplate('A'));
    expect(reg.has('a')).toBe(true);
    expect(reg.has('b')).toBe(false);
  });

  it('should return clone from get', () => {
    const reg = new TemplateRegistry();
    const tpl = new PromptTemplate('Hello {{.Name}}');
    reg.register('greet', tpl);
    const got = reg.get('greet')!;
    got.withVar('Name', 'Modified');
    // Original should not be modified
    expect(tpl.render()).not.toContain('Modified');
  });
});

// ===== PromptTemplate tests =====
describe('PromptTemplate', () => {
  it('should render with variables', () => {
    const tpl = new PromptTemplate('Hello {{.Name}}!');
    expect(tpl.withVar('Name', 'Alice').render()).toBe('Hello Alice!');
  });

  it('should render with multiple variables', () => {
    const tpl = new PromptTemplate('{{.Greeting}} {{.Name}}');
    expect(tpl.withVars({ Greeting: 'Hi', Name: 'Bob' }).render()).toBe('Hi Bob');
  });

  it('should set scope rules', () => {
    const tpl = new PromptTemplate('Rules: {{.ScopeRules}}');
    const result = tpl.withScopeRules(['/dir1', '/dir2']).render();
    expect(result).toContain('/dir1');
    expect(result).toContain('/dir2');
  });

  it('should clone', () => {
    const tpl = new PromptTemplate('Hello {{.Name}}').withVar('Name', 'Alice');
    const clone = tpl.clone();
    clone.withVar('Name', 'Bob');
    expect(tpl.render()).toBe('Hello Alice');
    expect(clone.render()).toBe('Hello Bob');
  });

  it('should handle missing variables', () => {
    const tpl = new PromptTemplate('Hello {{.Name}}!');
    expect(tpl.render()).toBe('Hello {{.Name}}!');
  });
});

describe('defaultSystemPrompt', () => {
  it('should create template with AgentName', () => {
    const tpl = defaultSystemPrompt();
    const result = tpl.withVar('AgentName', 'TestAgent').render();
    expect(result).toContain('TestAgent');
    expect(result).toContain('AI assistant');
  });
});

describe('codeAssistantTemplate', () => {
  it('should include scope rules', () => {
    const tpl = codeAssistantTemplate('You are a coder.', ['/src', '/lib']);
    const result = tpl.render();
    expect(result).toContain('You are a coder.');
    expect(result).toContain('/src');
    expect(result).toContain('/lib');
  });
});

describe('ragContextTemplate', () => {
  it('should create template with Context var', () => {
    const tpl = ragContextTemplate();
    const result = tpl.withVar('Context', 'some knowledge').render();
    expect(result).toContain('some knowledge');
    expect(result).toContain('Relevant Knowledge');
  });
});

describe('formatRAGDocuments', () => {
  it('should format documents', () => {
    const docs = [
      { id: '1', content: 'doc1', score: 0.9 },
      { id: '2', content: 'doc2', score: 0.8, role: 'user' },
    ];
    const result = formatRAGDocuments(docs);
    expect(result).toContain('doc1');
    expect(result).toContain('doc2');
    expect(result).toContain('0.90');
    expect(result).toContain('user');
  });

  it('should return empty for no docs', () => {
    expect(formatRAGDocuments([])).toBe('');
  });

  it('should use default role', () => {
    const docs = [{ id: '1', content: 'test', score: 0.5 }];
    const result = formatRAGDocuments(docs);
    expect(result).toContain('knowledge');
  });
});
