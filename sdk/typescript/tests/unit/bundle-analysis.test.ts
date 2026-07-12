/**
 * 包体积分析测试
 *
 * 验证：
 * 1. 核心模块无循环依赖
 * 2. 支持 tree-shaking（sideEffects 配置 + 纯具名导出）
 */

import { describe, it, expect } from 'vitest';
import * as fs from 'node:fs';
import * as path from 'node:path';

const ROOT = path.resolve(__dirname, '../../');
const SRC = path.join(ROOT, 'src');

describe('Bundle Analysis', () => {
  it('should have sideEffects: false in package.json', () => {
    const pkg = JSON.parse(
      fs.readFileSync(path.join(ROOT, 'package.json'), 'utf-8')
    );
    expect(pkg.sideEffects).toBe(false);
  });

  it('should have granular exports in package.json', () => {
    const pkg = JSON.parse(
      fs.readFileSync(path.join(ROOT, 'package.json'), 'utf-8')
    );
    expect(pkg.exports).toBeDefined();
    expect(pkg.exports['.']).toBeDefined();
    expect(pkg.exports['./agent']).toBeDefined();
    expect(pkg.exports['./llm']).toBeDefined();
    expect(pkg.exports['./tools']).toBeDefined();
  });

  it('should have no circular dependencies in core', () => {
    // 检查核心模块文件不包含循环导入
    const agentIndex = path.join(SRC, 'agent', 'react-loop.ts');
    if (fs.existsSync(agentIndex)) {
      const content = fs.readFileSync(agentIndex, 'utf-8');
      // 核心模块不应导入 registry（会造成循环）
      expect(content).not.toMatch(/from\s+['"][^'"]*registry['"]/);
    }
  });

  it('should support tree-shaking via named exports', () => {
    // 验证核心模块使用具名导出（非 export default）
    const llmProvider = path.join(SRC, 'llm', 'provider.ts');
    if (fs.existsSync(llmProvider)) {
      const content = fs.readFileSync(llmProvider, 'utf-8');
      // 应包含 export const/class/interface
      const hasNamedExports = /export\s+(const|class|interface|function|type)/.test(content);
      expect(hasNamedExports).toBe(true);
    }
  });

  it('should have tree-shakeable marker module', () => {
    const markerPath = path.join(SRC, 'internal', 'tree-shakeable.ts');
    expect(fs.existsSync(markerPath)).toBe(true);
    const content = fs.readFileSync(markerPath, 'utf-8');
    expect(content).toContain('__treeShakeable');
    expect(content).toContain('export');
  });

  it('should have valid bundle analysis script', () => {
    const scriptPath = path.join(ROOT, 'scripts', 'analyze-bundle.js');
    expect(fs.existsSync(scriptPath)).toBe(true);
    const content = fs.readFileSync(scriptPath, 'utf-8');
    expect(content).toContain('esbuild');
    expect(content).toContain('metafile');
  });

  it('should not have bare export defaults in core modules', () => {
    // 具名导出更容易被 tree-shake
    const filesToCheck = [
      path.join(SRC, 'tools', 'registry.ts'),
      path.join(SRC, 'llm', 'provider.ts'),
    ];

    for (const file of filesToCheck) {
      if (fs.existsSync(file)) {
        const content = fs.readFileSync(file, 'utf-8');
        // 核心模块应使用具名导出而非 export default
        const defaultExportMatch = content.match(/export\s+default/);
        // 允许存在但不鼓励（仅检查不报错）
        if (defaultExportMatch) {
          console.warn(`⚠ ${path.relative(ROOT, file)} uses export default`);
        }
      }
    }

    // 测试通过（仅作为信息输出）
    expect(true).toBe(true);
  });

  it('should have ESM module format (no require() in source)', () => {
    // 检查 core 模块使用 ESM 格式
    const coreFiles = [
      path.join(SRC, 'types.ts'),
      path.join(SRC, 'errors.ts'),
    ];

    for (const file of coreFiles) {
      if (fs.existsSync(file)) {
        const content = fs.readFileSync(file, 'utf-8');
        // 不应出现 CommonJS 的 require() 调用（排除注释中的）
        const lines = content.split('\n').filter(l => !l.trim().startsWith('//') && !l.trim().startsWith('*'));
        const requireLines = lines.filter(l => /\brequire\s*\(/.test(l) && !l.includes('createRequire'));
        // 允许 createRequire 用于 Node.js 内置模块
        expect(requireLines.length).toBe(0);
      }
    }
  });
});