package skills

import "encoding/json"

// Codec 技能序列化编解码器
type Codec struct{}

// NewCodec 创建编解码器
func NewCodec() *Codec {
	return &Codec{}
}

// Encode 将技能编码为 JSON
func (c *Codec) Encode(s *Skill) ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// Decode 从 JSON 解码技能
func (c *Codec) Decode(data []byte) (*Skill, error) {
	var s Skill
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// EncodeCompact 紧凑编码（单行 JSON）
func (c *Codec) EncodeCompact(s *Skill) ([]byte, error) {
	return json.Marshal(s)
}
