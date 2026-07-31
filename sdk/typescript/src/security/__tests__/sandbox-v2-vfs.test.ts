import { describe, it, expect, beforeEach } from 'vitest';
import { VirtualFS } from '../sandbox-v2.js';

describe('VirtualFS — 完整测试', () => {
  let vfs: VirtualFS;

  beforeEach(() => {
    vfs = new VirtualFS({ maxFileSize: 1024, maxTotalBytes: 4096 });
  });

  // ===== writeFile / readFile =====
  describe('writeFile / readFile 基本读写', () => {
    it('字符串写入后读回应一致', () => {
      vfs.writeFile('/hello.txt', 'hello world');
      expect(vfs.readTextFile('/hello.txt')).toBe('hello world');
    });

    it('Uint8Array 写入后读回应一致', () => {
      const data = new Uint8Array([0, 1, 2, 128, 255]);
      vfs.writeFile('/bin.dat', data);
      expect(vfs.readFile('/bin.dat')).toEqual(data);
    });

    it('覆盖写入应替换内容并更新 usedBytes', () => {
      vfs.writeFile('/f.txt', 'long content here');
      const after1 = vfs.getUsedBytes();
      vfs.writeFile('/f.txt', 'hi');
      expect(vfs.readTextFile('/f.txt')).toBe('hi');
      expect(vfs.getUsedBytes()).toBeLessThan(after1);
    });

    it('自动创建父目录', () => {
      vfs.writeFile('/a/b/c/deep.txt', 'deep');
      expect(vfs.exists('/a')).toBe(true);
      expect(vfs.exists('/a/b')).toBe(true);
      expect(vfs.exists('/a/b/c')).toBe(true);
      expect(vfs.readTextFile('/a/b/c/deep.txt')).toBe('deep');
    });

    it('写根目录应抛出错误', () => {
      expect(() => vfs.writeFile('/', 'data')).toThrow('Cannot write to root');
    });

    it('读取不存在文件应抛出 File not found', () => {
      expect(() => vfs.readFile('/nope')).toThrow('File not found');
    });

    it('readTextFile 对二进制数据应正确解码', () => {
      const text = '你好世界';
      vfs.writeFile('/utf8.txt', text);
      expect(vfs.readTextFile('/utf8.txt')).toBe(text);
    });
  });

  // ===== mkdir / listDir =====
  describe('mkdir / listDir 目录操作', () => {
    it('mkdir 创建多级目录', () => {
      vfs.mkdir('/x/y/z');
      expect(vfs.exists('/x/y/z')).toBe(true);
    });

    it('mkdir 根路径应无操作', () => {
      vfs.mkdir('/');
      expect(vfs.exists('/')).toBe(true);
    });

    it('listDir 返回排序后的条目', () => {
      vfs.writeFile('/c.txt', 'c');
      vfs.writeFile('/a.txt', 'a');
      vfs.mkdir('/b');
      const entries = vfs.listDir('/');
      expect(entries).toEqual(['a.txt', 'b', 'c.txt']);
    });

    it('listDir 空目录应返回空数组', () => {
      vfs.mkdir('/empty');
      expect(vfs.listDir('/empty')).toEqual([]);
    });

    it('listDir 非目录应抛出 Not a directory', () => {
      vfs.writeFile('/file.txt', 'data');
      expect(() => vfs.listDir('/file.txt')).toThrow('Not a directory');
    });

    it('listDir 不存在路径应抛出错误', () => {
      expect(() => vfs.listDir('/nonexistent')).toThrow('Not a directory');
    });
  });

  // ===== exists =====
  describe('exists 存在性检查', () => {
    it('文件和目录都应返回 true', () => {
      vfs.writeFile('/f.txt', 'data');
      vfs.mkdir('/dir');
      expect(vfs.exists('/f.txt')).toBe(true);
      expect(vfs.exists('/dir')).toBe(true);
    });

    it('不存在的路径返回 false', () => {
      expect(vfs.exists('/nothing')).toBe(false);
    });

    it('根路径始终存在', () => {
      expect(vfs.exists('/')).toBe(true);
    });
  });

  // ===== unlink =====
  describe('unlink 删除操作', () => {
    it('删除文件后 exists 返回 false 且 usedBytes 减少', () => {
      vfs.writeFile('/tmp.txt', '12345');
      const before = vfs.getUsedBytes();
      vfs.unlink('/tmp.txt');
      expect(vfs.exists('/tmp.txt')).toBe(false);
      expect(vfs.getUsedBytes()).toBe(before - 5);
    });

    it('删除目录（不检查空）', () => {
      vfs.mkdir('/dir');
      vfs.unlink('/dir');
      expect(vfs.exists('/dir')).toBe(false);
    });

    it('删除根应抛出 Cannot remove root', () => {
      expect(() => vfs.unlink('/')).toThrow('Cannot remove root');
    });

    it('删除不存在应抛出 Not found', () => {
      expect(() => vfs.unlink('/nope')).toThrow('Not found');
    });

    it('父目录不存在应抛出错误', () => {
      expect(() => vfs.unlink('/no/dir/file.txt')).toThrow('does not exist');
    });
  });

  // ===== 路径遍历 =====
  describe('路径遍历防护', () => {
    it('.. 应在虚拟根内解析而非逃逸', () => {
      vfs.writeFile('/a/b.txt', 'data');
      expect(vfs.exists('/a/../a/b.txt')).toBe(true);
    });

    it('多个 .. 应正确折叠', () => {
      vfs.writeFile('/top.txt', 'top');
      expect(vfs.exists('/a/b/../../top.txt')).toBe(true);
    });

    it('超过根目录的 .. 应折叠到根', () => {
      vfs.writeFile('/root_file.txt', 'here');
      // /../../ 折叠后仍为根
      expect(vfs.exists('/../../root_file.txt')).toBe(true);
    });

    it('. 路径应被忽略', () => {
      vfs.writeFile('/dir/./file.txt', 'data');
      expect(vfs.exists('/dir/file.txt')).toBe(true);
    });
  });

  // ===== 大小限制 =====
  describe('大小限制', () => {
    it('单文件超过 maxFileSize 应抛出', () => {
      const big = new Uint8Array(2048); // maxFileSize=1024
      expect(() => vfs.writeFile('/big.dat', big)).toThrow('exceeds max');
    });

    it('总量超过 maxTotalBytes 应抛出', () => {
      vfs.writeFile('/f1.dat', new Uint8Array(1024));
      vfs.writeFile('/f2.dat', new Uint8Array(1024));
      vfs.writeFile('/f3.dat', new Uint8Array(1024));
      vfs.writeFile('/f4.dat', new Uint8Array(1024));
      expect(() => vfs.writeFile('/f5.dat', new Uint8Array(1))).toThrow('exceed max');
    });

    it('覆盖写入时旧大小应被扣除', () => {
      vfs.writeFile('/f.dat', new Uint8Array(512));
      vfs.writeFile('/f.dat', new Uint8Array(256));
      expect(vfs.getUsedBytes()).toBe(256);
    });
  });

  // ===== 文件描述符 =====
  describe('文件描述符操作', () => {
    it('open + write + close + open + read 完整流程', () => {
      const fd = vfs.open('/fd.txt', { read: false, write: true, create: true, append: false, truncate: false });
      vfs.write(fd, new TextEncoder().encode('hello'));
      vfs.close(fd);

      const fd2 = vfs.open('/fd.txt', { read: true, write: false, create: false, append: false, truncate: false });
      const data = vfs.read(fd2, 100);
      expect(new TextDecoder().decode(data)).toBe('hello');
      vfs.close(fd2);
    });

    it('truncate 标志应清空文件内容', () => {
      vfs.writeFile('/t.txt', 'original');
      const fd = vfs.open('/t.txt', { read: true, write: true, create: false, append: false, truncate: true });
      expect(vfs.read(fd, 100).length).toBe(0);
      vfs.close(fd);
    });

    it('append 模式应从末尾写入', () => {
      vfs.writeFile('/a.txt', 'line1\n');
      const fd = vfs.open('/a.txt', { read: false, write: true, create: false, append: true, truncate: false });
      vfs.write(fd, new TextEncoder().encode('line2\n'));
      vfs.close(fd);
      expect(vfs.readTextFile('/a.txt')).toBe('line1\nline2\n');
    });

    it('read 在非读 fd 应抛出错误', () => {
      const fd = vfs.open('/w.txt', { read: false, write: true, create: true, append: false, truncate: false });
      expect(() => vfs.read(fd, 10)).toThrow('not opened for reading');
      vfs.close(fd);
    });

    it('write 在非写 fd 应抛出错误', () => {
      vfs.writeFile('/r.txt', 'data');
      const fd = vfs.open('/r.txt', { read: true, write: false, create: false, append: false, truncate: false });
      expect(() => vfs.write(fd, new Uint8Array(1))).toThrow('not opened for writing');
      vfs.close(fd);
    });

    it('无效 fd 应抛出 Invalid fd', () => {
      expect(() => vfs.read(999, 10)).toThrow('Invalid fd');
      expect(() => vfs.write(999, new Uint8Array(1))).toThrow('Invalid fd');
    });

    it('getFdPath 应返回正确路径', () => {
      const fd = vfs.open('/path.txt', { read: true, write: true, create: true, append: false, truncate: false });
      expect(vfs.getFdPath(fd)).toBe('/path.txt');
      expect(vfs.getFdPath(999)).toBeNull();
      vfs.close(fd);
    });

    it('open 非文件应抛出 Not a file', () => {
      vfs.mkdir('/dir');
      expect(() => vfs.open('/dir', { read: true, write: false, create: false, append: false, truncate: false })).toThrow('Not a file');
    });
  });

  // ===== reset =====
  describe('reset 清空行为', () => {
    it('应清空文件、目录、fd 和 usedBytes', () => {
      vfs.writeFile('/a.txt', 'data');
      vfs.mkdir('/dir');
      const fd = vfs.open('/a.txt', { read: true, write: false, create: false, append: false, truncate: false });
      vfs.reset();
      expect(vfs.exists('/a.txt')).toBe(false);
      expect(vfs.exists('/dir')).toBe(false);
      expect(vfs.getUsedBytes()).toBe(0);
      expect(() => vfs.read(fd, 10)).toThrow('Invalid fd');
    });
  });
});
