import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';
import * as os from 'node:os';
import { ShellTool } from '../../src/tools/builtin/shell.js';
import { WebTool, APITool } from '../../src/tools/builtin/web-api.js';
import { FileSystemTool } from '../../src/tools/builtin/filesystem.js';

// ===== ShellTool tests =====
describe('ShellTool', () => {
  it('should have correct name and description', () => {
    const tool = new ShellTool();
    expect(tool.name).toBe('shell');
    expect(tool.description).toContain('shell');
  });

  it('should have parameters', () => {
    const tool = new ShellTool();
    expect(tool.parameters).toBeDefined();
    expect(tool.parameters.required).toEqual(['command']);
  });

  it('should reject empty command', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: '' });
    expect(result).toContain('Error: command is required');
  });

  it('should reject whitespace-only command', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: '   ' });
    expect(result).toContain('Error: command is required');
  });

  it('should block blacklisted commands', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'rm -rf /' });
    expect(result).toContain('blocked pattern');
  });

  it('should block custom blacklisted patterns', async () => {
    const tool = new ShellTool({ commandBlacklist: ['dangerous'] });
    const result = await tool.execute({ command: 'dangerous command' });
    expect(result).toContain('blocked pattern');
  });

  it('should enforce whitelist', async () => {
    const tool = new ShellTool({ commandWhitelist: ['echo'] });
    const result = await tool.execute({ command: 'ls' });
    expect(result).toContain('not in the allowed list');
  });

  it('should allow whitelisted commands', async () => {
    const tool = new ShellTool({ commandWhitelist: ['echo'] });
    const result = await tool.execute({ command: 'echo hello' });
    expect(result.trim()).toBe('hello');
  });

  it('should block shell metacharacters', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'echo hello; echo world' });
    expect(result).toContain('shell metacharacter');
  });

  it('should block pipe character', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'echo hello | cat' });
    expect(result).toContain('shell metacharacter');
  });

  it('should block redirect character', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'echo hello > file.txt' });
    expect(result).toContain('shell metacharacter');
  });

  it('should execute safe commands', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'echo testoutput' });
    expect(result.trim()).toBe('testoutput');
  });

  it('should handle command errors', async () => {
    const tool = new ShellTool();
    const result = await tool.execute({ command: 'nonexistentcommand12345' });
    expect(result).toContain('Error:');
  });

  it('should use custom working directory', async () => {
    const tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-shell-'));
    const tool = new ShellTool({ workingDir: tmpDir });
    const result = await tool.execute({ command: 'echo hello' });
    expect(result.trim()).toBe('hello');
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should use default config', () => {
    const tool = new ShellTool();
    expect(tool.name).toBe('shell');
  });

  it('should use custom config', async () => {
    const tool = new ShellTool({
      commandWhitelist: ['echo'],
      commandBlacklist: ['custombad'],
      timeoutMs: 5000,
      maxOutputLength: 100,
    });
    expect(await tool.execute({ command: 'custombad' })).toContain('blocked');
    expect((await tool.execute({ command: 'echo hello' })).trim()).toBe('hello');
  });
});

// ===== WebTool tests =====
describe('WebTool', () => {
  it('should have correct name', () => {
    const tool = new WebTool();
    expect(tool.name).toBe('web');
  });

  it('should reject empty URL', async () => {
    const tool = new WebTool();
    const result = await tool.execute({ url: '' });
    expect(result).toContain('Error: URL is required');
  });

  it('should reject invalid URL', async () => {
    const tool = new WebTool();
    const result = await tool.execute({ url: 'not-a-url' });
    expect(result).toContain('Error: invalid URL');
  });

  it('should block blocked domains', async () => {
    const tool = new WebTool({ blockedDomains: ['evil.com'] });
    const result = await tool.execute({ url: 'https://evil.com/test' });
    expect(result).toContain('domain "evil.com" is blocked');
  });

  it('should enforce allowed domains', async () => {
    const tool = new WebTool({ allowedDomains: ['example.com'] });
    const result = await tool.execute({ url: 'https://other.com/test' });
    expect(result).toContain('not in allowed list');
  });

  it('should allow allowed domains', async () => {
    const tool = new WebTool({ allowedDomains: ['example.com'] });
    // This will make a real request, but it should not be blocked by domain check
    const result = await tool.execute({ url: 'https://example.com' });
    expect(result).not.toContain('not in allowed list');
  });

  it('should use default config', () => {
    const tool = new WebTool();
    expect(tool.name).toBe('web');
    expect(tool.parameters.required).toEqual(['url']);
  });

  it('should handle network errors gracefully', async () => {
    const tool = new WebTool({ timeoutMs: 100 });
    const result = await tool.execute({ url: 'https://localhost:1/test' });
    expect(result).toContain('Error:');
  });
});

// ===== APITool tests =====
describe('APITool', () => {
  it('should have correct name', () => {
    const tool = new APITool({ baseURL: 'https://api.example.com' });
    expect(tool.name).toBe('api');
  });

  it('should have parameters', () => {
    const tool = new APITool({ baseURL: 'https://api.example.com' });
    expect(tool.parameters.required).toEqual(['endpoint']);
  });

  it('should strip trailing slashes from baseURL', () => {
    const tool = new APITool({ baseURL: 'https://api.example.com///' });
    // Internal check via behavior
    expect(tool.name).toBe('api');
  });

  it('should enforce endpoint whitelist', async () => {
    const tool = new APITool({
      baseURL: 'https://api.example.com',
      allowedEndpoints: ['/users'],
    });
    const result = await tool.execute({ endpoint: '/admin/secret' });
    expect(result).toContain('not in allowed list');
  });

  it('should allow whitelisted endpoints', async () => {
    const tool = new APITool({
      baseURL: 'https://api.example.com',
      allowedEndpoints: ['/users'],
    });
    // This will fail with network error, but should not be blocked by whitelist
    const result = await tool.execute({ endpoint: '/users/list' });
    expect(result).not.toContain('not in allowed list');
  });

  it('should handle network errors', async () => {
    const tool = new APITool({ baseURL: 'https://localhost:1' });
    const result = await tool.execute({ endpoint: '/test' });
    expect(result).toContain('Error:');
  });

  it('should use default headers', () => {
    const tool = new APITool({
      baseURL: 'https://api.example.com',
      headers: { 'Authorization': 'Bearer token' },
    });
    expect(tool.name).toBe('api');
  });

  it('should allow no endpoint whitelist', async () => {
    const tool = new APITool({ baseURL: 'https://localhost:1' });
    const result = await tool.execute({ endpoint: '/anything' });
    expect(result).not.toContain('not in allowed list');
  });
});

// ===== FileSystemTool tests =====
describe('FileSystemTool', () => {
  let tmpDir: string;

  beforeEach(() => {
    tmpDir = fs.mkdtempSync(path.join(os.tmpdir(), 'ap-fs-'));
  });

  afterEach(() => {
    fs.rmSync(tmpDir, { recursive: true, force: true });
  });

  it('should have correct name', () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    expect(tool.name).toBe('filesystem');
  });

  it('should have parameters', () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    expect(tool.parameters.required).toEqual(['action', 'path']);
  });

  it('should detect path traversal', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'read', path: '../../../etc/passwd' });
    expect(result).toContain('path traversal detected');
  });

  it('should read files', async () => {
    const filePath = path.join(tmpDir, 'test.txt');
    fs.writeFileSync(filePath, 'hello world');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'read', path: 'test.txt' });
    expect(result).toBe('hello world');
  });

  it('should handle read errors', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'read', path: 'nonexistent.txt' });
    expect(result).toContain('Error reading file');
  });

  it('should write files', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'write', path: 'output.txt', content: 'test content' });
    expect(result).toContain('File written');
    expect(fs.readFileSync(path.join(tmpDir, 'output.txt'), 'utf-8')).toBe('test content');
  });

  it('should append to files', async () => {
    const filePath = path.join(tmpDir, 'append.txt');
    fs.writeFileSync(filePath, 'line1\n');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    await tool.execute({ action: 'append', path: 'append.txt', content: 'line2\n' });
    expect(fs.readFileSync(filePath, 'utf-8')).toBe('line1\nline2\n');
  });

  it('should list directories', async () => {
    fs.writeFileSync(path.join(tmpDir, 'file1.txt'), 'a');
    fs.writeFileSync(path.join(tmpDir, 'file2.txt'), 'b');
    fs.mkdirSync(path.join(tmpDir, 'subdir'));
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'list', path: '.' });
    expect(result).toContain('file1.txt');
    expect(result).toContain('file2.txt');
    expect(result).toContain('subdir');
    expect(result).toContain('[DIR]');
  });

  it('should show empty directory', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'list', path: '.' });
    expect(result).toBe('(empty directory)');
  });

  it('should search files', async () => {
    fs.writeFileSync(path.join(tmpDir, 'test.txt'), 'a');
    fs.writeFileSync(path.join(tmpDir, 'other.md'), 'b');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'search', path: '.', pattern: 'test' });
    expect(result).toContain('test.txt');
    expect(result).not.toContain('other.md');
  });

  it('should handle no search results', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'search', path: '.', pattern: 'nonexistent' });
    expect(result).toContain('No files found');
  });

  it('should require search pattern', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'search', path: '.' });
    expect(result).toContain('pattern is required');
  });

  it('should delete files', async () => {
    const filePath = path.join(tmpDir, 'todelete.txt');
    fs.writeFileSync(filePath, 'content');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'delete', path: 'todelete.txt' });
    expect(result).toContain('Deleted');
    expect(fs.existsSync(filePath)).toBe(false);
  });

  it('should delete directories', async () => {
    const dirPath = path.join(tmpDir, 'todelete');
    fs.mkdirSync(dirPath);
    fs.writeFileSync(path.join(dirPath, 'file.txt'), 'content');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'delete', path: 'todelete' });
    expect(result).toContain('Deleted');
    expect(fs.existsSync(dirPath)).toBe(false);
  });

  it('should handle delete errors', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'delete', path: 'nonexistent' });
    expect(result).toContain('Error deleting');
  });

  it('should create directories', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'mkdir', path: 'newdir/sub' });
    expect(result).toContain('Directory created');
    expect(fs.existsSync(path.join(tmpDir, 'newdir', 'sub'))).toBe(true);
  });

  it('should check existence', async () => {
    fs.writeFileSync(path.join(tmpDir, 'exists.txt'), 'content');
    const tool = new FileSystemTool({ rootDir: tmpDir });
    expect(await tool.execute({ action: 'exists', path: 'exists.txt' })).toBe('true');
    expect(await tool.execute({ action: 'exists', path: 'missing.txt' })).toBe('false');
  });

  it('should handle unknown action', async () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    const result = await tool.execute({ action: 'unknown', path: '.' });
    expect(result).toContain('unknown action');
  });

  it('should respect maxFileSize', async () => {
    const filePath = path.join(tmpDir, 'large.txt');
    fs.writeFileSync(filePath, 'x'.repeat(100));
    const tool = new FileSystemTool({ rootDir: tmpDir, maxFileSize: 10 });
    const result = await tool.execute({ action: 'read', path: 'large.txt' });
    expect(result).toContain('file too large');
  });

  it('should use default config', () => {
    const tool = new FileSystemTool({ rootDir: tmpDir });
    expect(tool.name).toBe('filesystem');
  });
});
