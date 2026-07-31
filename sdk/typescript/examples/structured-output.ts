/**
 * 结构化输出示例 — Zod schema 验证 LLM 响应
 *
 * 运行: npx tsx examples/structured-output.ts
 */
import { z } from 'zod';
import { StructuredOutput } from '../src/llm/structured-output.js';

// 定义输出 schema
const WeatherReport = z.object({
  city: z.string().describe('城市名称'),
  temperature: z.number().describe('温度（摄氏度）'),
  condition: z.enum(['sunny', 'cloudy', 'rainy', 'snowy']).describe('天气状况'),
  humidity: z.number().min(0).max(100).describe('湿度百分比'),
});

type WeatherReport = z.infer<typeof WeatherReport>;

async function main() {
  console.log('=== AgentPrimordia TS SDK: Structured Output (Zod) ===\n');

  const structured = new StructuredOutput(WeatherReport);

  // 模拟 LLM 返回的 JSON
  const validJson = '{"city": "北京", "temperature": 25, "condition": "sunny", "humidity": 60}';
  const invalidJson = '{"city": "上海", "temperature": "hot", "condition": "windy"}';

  // 验证有效输出
  console.log('--- Valid Response ---');
  const validResult = structured.parse(validJson);
  if (validResult.success) {
    console.log(`City: ${validResult.data.city}`);
    console.log(`Temperature: ${validResult.data.temperature}°C`);
    console.log(`Condition: ${validResult.data.condition}`);
    console.log(`Humidity: ${validResult.data.humidity}%`);
  }

  // 验证无效输出
  console.log('\n--- Invalid Response ---');
  const invalidResult = structured.parse(invalidJson);
  if (!invalidResult.success) {
    console.log(`Validation failed with ${invalidResult.errors.length} error(s):`);
    for (const err of invalidResult.errors) {
      console.log(`  - ${err.path}: ${err.message}`);
    }
  }

  // 生成 JSON Schema（供 LLM function calling）
  console.log('\n--- Generated JSON Schema ---');
  const schema = structured.toJSONSchema();
  console.log(JSON.stringify(schema, null, 2).slice(0, 200) + '...');

  console.log('\n--- Done ---');
}

main().catch(console.error);
