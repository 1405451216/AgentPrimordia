// Package tools 内置tool系统
//
// shellcheck.go 提供 shell 元字符检测等纯函数安全检查，
// 供 builtin tool（shell/filesystem）和 security 包复用，
// 避免下层包反向依赖横向支撑层。
package tools

import "strings"

// dangerousChars 包含常见的 shell 元字符，用于检测命令注入攻击。
// 这些字符在 shell 中具有特殊语义，可用于拼接额外命令或控制流：
//
//	;  : 命令分隔符 (cmd1; cmd2)
//	|  : 管道 (cmd1 | cmd2)
//	&  : 后台执行 / 命令链接 (cmd1 & cmd2)
//	$  : 变量展开 / 命令替换 ($(cmd))
//	`  : 命令替换 (`cmd`)
//	> < : 重定向
//	\n\r: 换行符（可注入第二行命令）
//	() : 子 shell / 命令分组
var dangerousChars = []string{";", "|", "&", "$", "`", ">", "<", "\n", "\r", "(", ")"}

// ContainsShellMetacharacter 检查命令字符串是否包含 shell 元字符。
// 返回 (true, 元字符) 或 (false, "")。用于 Shell tool和安全沙箱的统一校验。
func ContainsShellMetacharacter(cmd string) (bool, string) {
	for _, ch := range dangerousChars {
		if strings.Contains(cmd, ch) {
			return true, ch
		}
	}
	return false, ""
}
