const { readFileSync, writeFileSync } = require("fs");
const path = "E:/ap/AgentPrimordia/sdk/typescript/tests/unit/turn-executor.test.ts";
let content = readFileSync(path, "utf-8");
const before = content.length;
// Remove all \r characters
content = content.replace(/\r/g, "");
const after = content.length;
writeFileSync(path, content, "utf-8");
console.log(`Removed ${before - after} CR chars. Now ${content.split("\n").length} lines.`);
