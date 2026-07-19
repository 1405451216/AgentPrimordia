/**
 * 复制非 TypeScript 资产到 dist/ 目录
 *
 * 原因：tsc 只负责编译 .ts → .js，不会复制 HTML / CSS 等静态文件。
 * 本脚本把 src/{popup,devtools} 中的 .html / .css 镜像到 dist/ 对应位置。
 */
import { readdirSync, statSync, copyFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, extname, relative, resolve } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const SRC_DIR = resolve(__dirname, '..', 'src');
const DIST_DIR = resolve(__dirname, '..', 'dist');

/** 递归复制指定扩展名的文件，保持目录结构 */
function copyByExt(srcDir, destDir, extensions) {
    for (const entry of readdirSync(srcDir, { withFileTypes: true })) {
        const srcPath = join(srcDir, entry.name);
        if (entry.isDirectory()) {
            copyByExt(srcPath, join(destDir, entry.name), extensions);
        } else if (extensions.includes(extname(entry.name))) {
            const relPath = relative(SRC_DIR, srcPath);
            const destPath = join(DIST_DIR, relPath);
            mkdirSync(dirname(destPath), { recursive: true });
            copyFileSync(srcPath, destPath);
            console.log(`  ✓ ${relPath}`);
        }
    }
}

console.log('Copying static assets → dist/...');
copyByExt(SRC_DIR, DIST_DIR, ['.html', '.css']);
console.log('Done.');
