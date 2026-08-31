// exporter.go — 蒸馏数据集导出（管道第三段：轨迹 → ap-dataset-v1 工件）
//
// 格式契约（命题 2「权重标准格式落盘」，双线对账面）：
//   - JSONL：每行一条 DistillationExample（确定性序列化，行序 = 输入序）；
//   - Manifest：sha256 对 JSONL 全文字节复算，manifest_id 取哈希前 16 位；
//   - 确定性：同输入集必得字节级相同的 JSONL 与 manifest——TS 消费端
//     （sdk/typescript/src/learning/pipeline.ts）以同一契约校验。
package pipeline

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// FormatVersion 数据集格式版本（契约锁定；变更须主版本评审）。
const FormatVersion = "ap-dataset-v1"

// Export 将候选轨迹转为聊天格式数据集并计算清单。
// 确定性：输出行序 = candidates 输入序；同输入必得同 manifest。
func Export(candidates []CuratedCandidate, domain, source string, at time.Time) (*Dataset, error) {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	var b strings.Builder
	for _, c := range candidates {
		ex := DistillationExample{
			ID:       c.Trajectory.ID,
			Domain:   c.Trajectory.Domain,
			Messages: toDatasetMessages(c.Trajectory),
			Weight:   c.Weight,
		}
		line, err := json.Marshal(ex)
		if err != nil {
			return nil, fmt.Errorf("pipeline: 样例 %s 序列化失败: %w", ex.ID, err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	jsonl := []byte(b.String())
	sum := sha256.Sum256(jsonl)
	shaHex := hex.EncodeToString(sum[:])
	manifest := DatasetManifest{
		FormatVersion: FormatVersion,
		ManifestID:    shaHex[:16],
		Domain:        domain,
		Count:         len(candidates),
		SHA256:        shaHex,
		Bytes:         len(jsonl),
		CreatedAt:     at.UTC(),
		Source:        source,
	}
	return &Dataset{Manifest: manifest, JSONL: jsonl}, nil
}

// VerifyDataset 消费端校验：JSONL 字节与清单互证（TS 侧同算法）。
// 返回 error = 数据集与清单不符（损坏/篡改/版本漂移）。
func VerifyDataset(d *Dataset) error {
	m := d.Manifest
	if m.FormatVersion != FormatVersion {
		return fmt.Errorf("pipeline: 数据集格式版本 %s ≠ %s", m.FormatVersion, FormatVersion)
	}
	sum := sha256.Sum256(d.JSONL)
	shaHex := hex.EncodeToString(sum[:])
	if shaHex != m.SHA256 {
		return fmt.Errorf("pipeline: JSONL sha256 %s ≠ 清单登记 %s", shaHex, m.SHA256)
	}
	if len(d.JSONL) != m.Bytes {
		return fmt.Errorf("pipeline: JSONL 字节数 %d ≠ 清单登记 %d", len(d.JSONL), m.Bytes)
	}
	lines := strings.Count(strings.TrimRight(string(d.JSONL), "\n"), "\n") + 1
	if strings.TrimSpace(string(d.JSONL)) == "" {
		lines = 0
	}
	if lines != m.Count {
		return fmt.Errorf("pipeline: JSONL 行数 %d ≠ 清单登记 %d", lines, m.Count)
	}
	if m.ManifestID != m.SHA256[:16] {
		return fmt.Errorf("pipeline: manifest_id %s ≠ sha256 前 16 位 %s", m.ManifestID, m.SHA256[:16])
	}
	return nil
}

// ParseDataset 解析 JSONL 为样例列表（消费端：训练连接器/影子评测/TS 消费者）。
func ParseDataset(jsonl []byte) ([]DistillationExample, error) {
	var out []DistillationExample
	for i, line := range strings.Split(strings.TrimRight(string(jsonl), "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var ex DistillationExample
		if err := json.Unmarshal([]byte(line), &ex); err != nil {
			return nil, fmt.Errorf("pipeline: 第 %d 行解析失败: %w", i+1, err)
		}
		out = append(out, ex)
	}
	return out, nil
}

// toDatasetMessages 轨迹轮次 → 聊天格式消息（确定性映射）。
// callIdx 约定：第 k 个含工具调用的 assistant 轮发出 call-k；其后紧跟的
// tool 轮回填同一 call-k 与工具名——契约上 assistant 调用与 tool 结果
// 必须成对可关联（训练端点按此配对构建监督信号）。
func toDatasetMessages(t Trajectory) []DatasetMessage {
	msgs := make([]DatasetMessage, 0, len(t.Turns))
	callIdx := 0
	for _, tt := range t.Turns {
		switch tt.Role {
		case "tool":
			msgs = append(msgs, DatasetMessage{
				Role:       "tool",
				Content:    tt.Observation,
				ToolCallID: fmt.Sprintf("call-%d", callIdx),
				Name:       assistantToolName(t, callIdx),
			})
		default:
			msg := DatasetMessage{Role: tt.Role, Content: tt.Content}
			if tt.Role == "assistant" && (tt.ToolName != "" || len(tt.ToolCalls) > 0) {
				callIdx++
				msg.ToolCalls = toolCallsJSON(tt, callIdx)
			}
			msgs = append(msgs, msg)
		}
	}
	return msgs
}

// toolCallsJSON 把 assistant 轮的工具调用序列化为 JSON 数组文本。
func toolCallsJSON(tt TrajectoryTurn, callIdx int) string {
	type call struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	}
	var calls []call
	if tt.ToolName != "" {
		calls = append(calls, call{ID: fmt.Sprintf("call-%d", callIdx), Name: tt.ToolName, Arguments: tt.ToolArgs})
	}
	for _, summary := range tt.ToolCalls {
		calls = append(calls, call{ID: fmt.Sprintf("call-%d", callIdx), Name: summary, Arguments: ""})
	}
	if len(calls) == 0 {
		return ""
	}
	b, err := json.Marshal(calls)
	if err != nil {
		return ""
	}
	return string(b)
}

// assistantToolName 回溯第 callIdx 个含工具调用的 assistant 轮的工具名。
func assistantToolName(t Trajectory, callIdx int) string {
	seq := 0
	for _, tt := range t.Turns {
		if tt.Role == "assistant" && (tt.ToolName != "" || len(tt.ToolCalls) > 0) {
			seq++
			if seq == callIdx {
				if tt.ToolName != "" {
					return tt.ToolName
				}
				if len(tt.ToolCalls) > 0 {
					return tt.ToolCalls[0]
				}
				return ""
			}
		}
	}
	return ""
}
