package registry

import (
	"crypto/rand"
	"encoding/json"
)

// randReader 返回密码学安全的随机数源（便于测试覆盖）。
func randReader() interface {
	Read(p []byte) (n int, err error)
} {
	return rand.Reader
}

// jsonStdUnmarshal 是 encoding/json.Unmarshal 的别名，便于未来替换为更快的 JSON 解析器。
func jsonStdUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
