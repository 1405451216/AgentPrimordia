// ESLint 9 flat config — AgentPrimordia TypeScript SDK
import js from '@eslint/js';
import tseslint from 'typescript-eslint';

export default tseslint.config(
  // 基础规则
  js.configs.recommended,

  // TypeScript 推荐规则（类型感知）
  ...tseslint.configs.recommended,

  // 项目特定配置
  {
    files: ['src/**/*.ts'],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: 'module',
    },
    rules: {
      // 禁止 any（允许 unknown）
      '@typescript-eslint/no-explicit-any': 'warn',
      // 禁止未使用的变量（忽略前缀为 _ 的参数）
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_', varsIgnorePattern: '^_', caughtErrorsIgnorePattern: '^_|err|error' }],
      // 强制一致的类型导入
      '@typescript-eslint/consistent-type-imports': ['error', { prefer: 'type-imports' }],
      // 禁止 require()（使用 ESM import）
      '@typescript-eslint/no-require-imports': 'error',
      // 允许 async 函数不需要 await（回调场景常见）
      '@typescript-eslint/no-misused-promises': 'off',
      // 循环中的 await（Agent 循环需要顺序执行）
      'no-await-in-loop': 'off',
      // 控制台输出（SDK 中合理使用）
      'no-console': 'off',
      // 允许空 catch 块（TS 中常见的错误忽略模式）
      'no-empty': ['error', { allowEmptyCatch: true }],
    },
  },

  // 测试文件配置（宽松规则）
  {
    files: ['tests/**/*.ts'],
    rules: {
      '@typescript-eslint/no-explicit-any': 'off',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      'no-console': 'off',
    },
  },

  // 忽略文件
  {
    ignores: ['dist/', 'node_modules/', 'coverage/', '*.config.*'],
  },
);
