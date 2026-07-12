const { readFileSync, writeFileSync } = require("fs");
const path = "E:/ap/AgentPrimordia/sdk/typescript/tests/unit/turn-executor.test.ts";
// The file has NO newlines now - everything is on one line separated by \r
// We need to read as bytes and reconstruct
const content = readFileSync(path, "utf-8");
// Check what we have
const crCount = (content.match(/\r/g) || []).length;
const lfCount = (content.match(/\n/g) || []).length;
console.log(`CR: ${crCount}, LF: ${lfCount}, total length: ${content.length}`);
// The \r chars were removed. The file is now broken because \r was the only line separator.
// We need to recover. Let's see if git has an original.
