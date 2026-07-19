/**
 * 用 Node.js 内置模块生成 AgentPrimordia 扩展图标（PNG）
 *
 * 不依赖任何第三方库：直接构造 PNG 文件格式 + zlib.deflateSync 压缩。
 * 输出：icons/icon16.png / icon48.png / icon128.png
 *
 * 设计：紫色渐变圆角方块 + 白色 "AP" 文字（字号按尺寸缩放）。
 */
import { deflateSync } from 'node:zlib';
import { writeFileSync, mkdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const __dirname = dirname(fileURLToPath(import.meta.url));
const ICONS_DIR = join(__dirname, '..', 'icons');

/** 简单 8x12 字模：A 与 P（1 = 亮，0 = 暗） */
const GLYPHS = {
    A: [
        '00011000',
        '00111100',
        '01100110',
        '01100110',
        '01100110',
        '01111110',
        '01111110',
        '01100110',
        '01100110',
        '01100110',
        '00000000',
        '00000000',
    ],
    P: [
        '01111100',
        '01111110',
        '01100110',
        '01100110',
        '01111110',
        '01111100',
        '01100000',
        '01100000',
        '01100000',
        '01100000',
        '00000000',
        '00000000',
    ],
};

/** CRC32 计算 */
function crc32(buf) {
    let c = ~0;
    for (let i = 0; i < buf.length; i++) {
        c ^= buf[i];
        for (let k = 0; k < 8; k++) c = (c >>> 1) ^ (0xedb88320 & -(c & 1));
    }
    return ~c >>> 0;
}

/** 构造一个 PNG chunk */
function chunk(type, data) {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const typeBuf = Buffer.from(type, 'ascii');
    const body = Buffer.concat([typeBuf, data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(crc32(body));
    return Buffer.concat([len, body, crc]);
}

/** 将 RGBA 像素数组编码为 PNG Buffer */
function encodePNG(width, height, rgba) {
    const sig = Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]);

    // IHDR
    const ihdr = Buffer.alloc(13);
    ihdr.writeUInt32BE(width, 0);
    ihdr.writeUInt32BE(height, 4);
    ihdr.writeUInt8(8, 8); // bit depth
    ihdr.writeUInt8(6, 9); // color type RGBA
    ihdr.writeUInt8(0, 10); // compression
    ihdr.writeUInt8(0, 11); // filter
    ihdr.writeUInt8(0, 12); // interlace

    // 原始扫描线：每行前面加 filter byte (0 = none)
    const raw = Buffer.alloc(height * (1 + width * 4));
    for (let y = 0; y < height; y++) {
        raw[y * (1 + width * 4)] = 0;
        rgba.copy(raw, y * (1 + width * 4) + 1, y * width * 4, (y + 1) * width * 4);
    }
    const idat = deflateSync(raw);

    return Buffer.concat([
        sig,
        chunk('IHDR', ihdr),
        chunk('IDAT', idat),
        chunk('IEND', Buffer.alloc(0)),
    ]);
}

/** 颜色插值 */
function lerpColor(c1, c2, t) {
    return [
        Math.round(c1[0] + (c2[0] - c1[0]) * t),
        Math.round(c1[1] + (c2[1] - c1[1]) * t),
        Math.round(c1[2] + (c2[2] - c1[2]) * t),
    ];
}

/** 用指定尺寸绘制图标像素并返回 RGBA buffer */
function drawIcon(size) {
    const rgba = Buffer.alloc(size * size * 4);
    const c1 = [0x6c, 0x5c, 0xe7]; // #6C5CE7
    const c2 = [0xa2, 0x9b, 0xfe]; // #a29bfe
    const radius = Math.floor(size * 0.18);

    // 文字区域
    const charW = 8;
    const charH = 12;
    const scale = Math.max(1, Math.floor(size / 32));
    const gap = scale;
    const textW = charW * scale * 2 + gap;
    const textH = charH * scale;
    const offX = Math.floor((size - textW) / 2);
    const offY = Math.floor((size - textH) / 2);

    const setPixel = (x, y, rgb, a = 255) => {
        const idx = (y * size + x) * 4;
        rgba[idx] = rgb[0];
        rgba[idx + 1] = rgb[1];
        rgba[idx + 2] = rgb[2];
        rgba[idx + 3] = a;
    };

    /** 检测是否在圆角矩形内 */
    const inRounded = (x, y) => {
        if (x < 0 || y < 0 || x >= size || y >= size) return false;
        const r = radius;
        if (x < r && y < r && Math.hypot(x - r, y - r) > r) return false;
        if (x >= size - r && y < r && Math.hypot(x - (size - 1 - r), y - r) > r) return false;
        if (x < r && y >= size - r && Math.hypot(x - r, y - (size - 1 - r)) > r) return false;
        if (x >= size - r && y >= size - r && Math.hypot(x - (size - 1 - r), y - (size - 1 - r)) > r) return false;
        return true;
    };

    /** 检测是否在字符像素上 */
    const inGlyph = (x, y) => {
        const gx = x - offX;
        const gy = y - offY;
        if (gx < 0 || gy < 0) return false;
        const cx = Math.floor(gx / scale);
        const cy = Math.floor(gy / scale);
        let chIdx;
        if (cx < charW) chIdx = 0;
        else if (cx < charW + gap / scale) return false;
        else if (cx < charW * 2 + gap / scale) chIdx = 1;
        else return false;
        const localX = chIdx === 0 ? cx : cx - charW - Math.floor(gap / scale);
        if (cy >= charH || localX >= charW) return false;
        const glyph = chIdx === 0 ? GLYPHS.A : GLYPHS.P;
        return glyph[cy][localX] === '1';
    };

    for (let y = 0; y < size; y++) {
        for (let x = 0; x < size; x++) {
            if (!inRounded(x, y)) {
                setPixel(x, y, [0, 0, 0], 0); // 透明
                continue;
            }
            const t = (x + y) / (2 * size);
            const color = lerpColor(c1, c2, t);
            if (inGlyph(x, y)) {
                setPixel(x, y, [255, 255, 255]); // 白字
            } else {
                setPixel(x, y, color);
            }
        }
    }

    return rgba;
}

/** 生成并写入单个图标 */
function generate(size, filename) {
    const rgba = drawIcon(size);
    const png = encodePNG(size, size, rgba);
    const outPath = join(ICONS_DIR, filename);
    writeFileSync(outPath, png);
    console.log(`✓ ${filename} (${size}×${size}, ${png.length} bytes)`);
}

mkdirSync(ICONS_DIR, { recursive: true });
for (const size of [16, 48, 128]) {
    generate(size, `icon${size}.png`);
}
console.log('All icons generated.');
