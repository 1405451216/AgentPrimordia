# Tools

## ToolRegistry

Register and execute tools for agent use.

```typescript
import { ToolRegistry, Tool } from '@agentprimordia/sdk';

const registry = new ToolRegistry();

// Define a tool
class CalculatorTool implements Tool {
  name = 'calculator';
  description = 'Perform basic arithmetic';
  parameters = {
    type: 'object' as const,
    properties: {
      expression: { type: 'string', description: 'Math expression to evaluate' },
    },
    required: ['expression'],
  };

  async execute(args: { expression: string }): Promise<string> {
    try {
      const result = Function(`"use strict"; return (${args.expression})`)();
      return String(result);
    } catch {
      return 'Error: Invalid expression';
    }
  }
}

registry.register(new CalculatorTool());
```

## Tool Execution

```typescript
// Execute a tool call
const result = await registry.execute({
  id: 'call_1',
  name: 'calculator',
  arguments: '{ "expression": "2 + 3" }',
});
console.log(result.content); // "5"
```

## OpenAI-Compatible Definitions

The registry generates tool definitions compatible with OpenAI's function calling format:

```typescript
const definitions = registry.definitions();
// [
//   {
//     type: 'function',
//     function: { name: 'calculator', description: '...', parameters: {...} }
//   }
// ]
```

## FileScopePolicy

Manage file access scopes for multi-agent setups.

```typescript
import { FileScopePolicy } from '@agentprimordia/sdk';

const policy = new FileScopePolicy();

// Set scope for agent
policy.setScope('agent-1', ['/data/agent1', '/shared/read']);

// Check access
policy.allow('agent-1', '/data/agent1/file.txt'); // true
policy.allow('agent-1', '/data/agent2/file.txt'); // false

// Validate no overlapping scopes
const error = policy.validate({
  'agent-1': ['/data/shared'],
  'agent-2': ['/data/shared'],
});
```
