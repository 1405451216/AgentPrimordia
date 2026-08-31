// wasmbin_test.go — 确定性 WASM 二进制夹具构造器（边界断言套件用）
//
// 全部按 WASM 二进制格式手工组装（无外部工具依赖），字节级确定性——
// 红队回归（A5/A6/A7/A8）的固定输入。
package wasm

import "testing"

// lebU32 无符号 LEB128 编码。
func lebU32(v uint32) []byte {
	var out []byte
	for {
		b := byte(v & 0x7F)
		v >>= 7
		if v != 0 {
			b |= 0x80
		}
		out = append(out, b)
		if v == 0 {
			return out
		}
	}
}

// section 组装一个段（id + 内容长度 + 内容）。
func section(id byte, content []byte) []byte {
	out := []byte{id}
	out = append(out, lebU32(uint32(len(content)))...)
	return append(out, content...)
}

// wasmHeader 模块头。
var wasmHeader = []byte{0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00}

// toolWasm 构造满足 ExecuteWithMemory 协议的最小工具模块：
//
//	(module
//	  (memory (export "memory") 1)
//	  (global $g (mut i64) i64.const 1024)
//	  (func (export "alloc") (param i64) (result i64)
//	    global.get $g   local.get 0   i64.add   global.set $g   global.get $g)
//	  (func (export "tool_execute") (param i64 i64) (result i64 i64)
//	    local.get 0   local.get 1))
func toolWasm(t *testing.T) []byte {
	t.Helper()
	out := append([]byte(nil), wasmHeader...)

	// type: [ (i64)->i64, (i64,i64)->(i64,i64) ]
	types := []byte{0x02,
		0x60, 0x01, 0x7E, 0x01, 0x7E,
		0x60, 0x02, 0x7E, 0x7E, 0x02, 0x7E, 0x7E}
	out = append(out, section(0x01, types)...)

	// func: func0=type0(alloc), func1=type1(tool_execute)
	out = append(out, section(0x03, []byte{0x02, 0x00, 0x01})...)

	// memory: min 1
	out = append(out, section(0x05, []byte{0x01, 0x00, 0x01})...)

	// global: mut i64 = 1024（LEB128 有符号：0x80 0x08）
	globals := []byte{0x01, 0x7E, 0x01, 0x42, 0x80, 0x08, 0x0B}
	out = append(out, section(0x06, globals)...)

	// export: memory/alloc/tool_execute
	exports := []byte{0x03,
		0x06, 'm', 'e', 'm', 'o', 'r', 'y', 0x02, 0x00,
		0x05, 'a', 'l', 'l', 'o', 'c', 0x00, 0x00,
		0x0C, 't', 'o', 'o', 'l', '_', 'e', 'x', 'e', 'c', 'u', 't', 'e', 0x00, 0x01}
	out = append(out, section(0x07, exports)...)

	// code:
	//   alloc: locals 00; 23 00 (global.get 0); 20 00; 7C (i64.add); 24 00; 23 00; 0B
	//   tool_execute: locals 00; 20 00; 20 01; 0B
	body0 := []byte{0x00, 0x23, 0x00, 0x20, 0x00, 0x7C, 0x24, 0x00, 0x23, 0x00, 0x0B}
	body1 := []byte{0x00, 0x20, 0x00, 0x20, 0x01, 0x0B}
	code := []byte{0x02}
	code = append(code, lebU32(uint32(len(body0)))...)
	code = append(code, body0...)
	code = append(code, lebU32(uint32(len(body1)))...)
	code = append(code, body1...)
	out = append(out, section(0x0A, code)...)

	return out
}

// spinWasm 构造无限循环模块（导出 "spin"：loop br 0）——A5 CPU 配额终止用。
func spinWasm(t *testing.T) []byte {
	t.Helper()
	out := append([]byte(nil), wasmHeader...)
	// type: ()->()
	out = append(out, section(0x01, []byte{0x01, 0x60, 0x00, 0x00})...)
	// func: func0=type0
	out = append(out, section(0x03, []byte{0x01, 0x00})...)
	// export: "spin" func 0
	out = append(out, section(0x07, []byte{0x01, 0x04, 's', 'p', 'i', 'n', 0x00, 0x00})...)
	// code: locals 00; 03 40 (loop); 0C 00 (br 0); 0B (end loop); 0B (end func)
	out = append(out, section(0x0A, []byte{0x01, 0x07, 0x00, 0x03, 0x40, 0x0C, 0x00, 0x0B, 0x0B})...)
	return out
}

// wasiImportWasm 构造导入 wasi_snapshot_preview1.fd_write 的模块——A7 负样本。
func wasiImportWasm(t *testing.T) []byte {
	t.Helper()
	out := append([]byte(nil), wasmHeader...)
	// type: ()->()
	out = append(out, section(0x01, []byte{0x01, 0x60, 0x00, 0x00})...)
	// import: wasi_snapshot_preview1.fd_write -> func 0（type 0）
	modName := "wasi_snapshot_preview1"
	field := "fd_write"
	imp := []byte{0x01}
	imp = append(imp, lebU32(uint32(len(modName)))...)
	imp = append(imp, modName...)
	imp = append(imp, lebU32(uint32(len(field)))...)
	imp = append(imp, field...)
	imp = append(imp, 0x00, 0x00)
	out = append(out, section(0x02, imp)...)
	// 仅导入、无自定义函数段（函数段与代码段数量必须一致；纯导入模块合法）
	return out
}
