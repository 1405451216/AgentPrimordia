package a2a

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// ===== 认证相关 =====

type AuthType string

const (
	AuthNone   AuthType = "none"
	AuthAPIKey AuthType = "api_key"
	AuthBearer AuthType = "bearer"
	AuthMTLS   AuthType = "mtls"
)

type SecurityScheme struct {
	Scheme AuthType `json:"scheme"`
	In     string   `json:"in"`
	Name   string   `json:"name"`
	Scopes []string `json:"scopes,omitempty"`
}

// ===== AgentCard =====

type AgentSkill struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	InputModes  []string `json:"input_modes,omitempty"`
	OutputModes []string `json:"output_modes,omitempty"`
}

type AgentCapabilities struct {
	InputModes  []string `json:"input_modes"`
	OutputModes []string `json:"output_modes"`
	Streaming   bool     `json:"streaming"`
}

type AgentEndpoints struct {
	BaseURL       string `json:"base_url"`
	TaskSend      string `json:"task_send"`
	TaskGet       string `json:"task_get"`
	TaskCancel    string `json:"task_cancel"`
	TaskSubscribe string `json:"task_subscribe"`
	AgentCardURL  string `json:"agent_card_url,omitempty"`
}

type AgentCard struct {
	Protocol        string            `json:"protocol"`
	AgentID         string            `json:"agent_id"`
	Name            string            `json:"name"`
	Description     string            `json:"description,omitempty"`
	Capabilities    AgentCapabilities `json:"capabilities"`
	Endpoints       AgentEndpoints    `json:"endpoints"`
	SecuritySchemes []SecurityScheme  `json:"security_schemes"`
	Skills          []AgentSkill      `json:"skills,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

func (c *AgentCard) MarshalJSON() ([]byte, error) {
	type Alias AgentCard
	return json.Marshal((*Alias)(c))
}
func (c *AgentCard) UnmarshalJSON(data []byte) error {
	type Alias AgentCard
	return json.Unmarshal(data, (*Alias)(c))
}

func NewAgentCard(agentID, name string) *AgentCard {
	return &AgentCard{
		Protocol: "a2a",
		AgentID:  agentID,
		Name:     name,
		Capabilities: AgentCapabilities{
			InputModes:  []string{"text"},
			OutputModes: []string{"text"},
			Streaming:   true,
		},
	}
}

// ===== Task 状态机 =====

type TaskState string

const (
	TaskSubmitted     TaskState = "submitted"
	TaskWorking       TaskState = "working"
	TaskInputRequired TaskState = "input-required"
	TaskCompleted     TaskState = "completed"
	TaskFailed        TaskState = "failed"
	TaskCanceled      TaskState = "canceled"
	TaskRejected      TaskState = "rejected"
)

var validTransitions = map[TaskState][]TaskState{
	TaskSubmitted:     {TaskWorking, TaskRejected, TaskCanceled},
	TaskWorking:       {TaskCompleted, TaskFailed, TaskCanceled, TaskInputRequired},
	TaskInputRequired: {TaskWorking, TaskCanceled},
}

func IsValidTransition(from, to TaskState) bool {
	allowed, ok := validTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func IsTerminal(state TaskState) bool {
	switch state {
	case TaskCompleted, TaskFailed, TaskCanceled, TaskRejected:
		return true
	default:
		return false
	}
}

type TaskStatus struct {
	State         TaskState   `json:"state"`
	ErrorMessage  string      `json:"error_message,omitempty"`
	StreamMessage *A2AMessage `json:"stream_message,omitempty"`
}

type Task struct {
	ID        string      `json:"id"`
	SessionID string      `json:"session_id,omitempty"`
	State     TaskState   `json:"state"`
	Message   *A2AMessage `json:"message"`
	Status    *TaskStatus `json:"status,omitempty"`
	Artifacts []Artifact  `json:"artifacts,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
	ExpiresAt time.Time   `json:"expires_at,omitempty"`
}

// ===== 消息与 Parts =====

type Part interface {
	Type() string
}

type TextPart struct {
	TypeField string `json:"type"`
	Text      string `json:"text"`
}

func NewTextPart(text string) TextPart { return TextPart{TypeField: "text", Text: text} }
func (t TextPart) Type() string        { return t.TypeField }

type FileWithBytes struct {
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Bytes    string `json:"bytes"`
}

type FileWithURI struct {
	URI      string `json:"uri"`
	MimeType string `json:"mime_type"`
}

type FilePart struct {
	TypeField string         `json:"type"`
	File      *FileWithBytes `json:"file,omitempty"`
	FileURI   *FileWithURI   `json:"file_uri,omitempty"`
	MimeType  string         `json:"mimetype"`
	Filename  string         `json:"filename,omitempty"`
}

func NewFilePartFromURI(uri, mime string) FilePart {
	return FilePart{TypeField: "file", FileURI: &FileWithURI{URI: uri, MimeType: mime}}
}
func (f FilePart) Type() string { return f.TypeField }

type DataPart struct {
	TypeField string          `json:"type"`
	Data      json.RawMessage `json:"data"`
}

func NewDataPart(data json.RawMessage) DataPart { return DataPart{TypeField: "data", Data: data} }
func (d DataPart) Type() string                 { return d.TypeField }

type A2AMessage struct {
	Role      string `json:"role"`
	Parts     []Part `json:"parts"`
	MessageID string `json:"message_id,omitempty"`
	ParentID  string `json:"parent_id,omitempty"`
}

func (m *A2AMessage) MarshalJSON() ([]byte, error) {
	type Alias A2AMessage
	raw := struct {
		Parts []json.RawMessage `json:"parts"`
		*Alias
	}{Alias: (*Alias)(m)}
	for _, p := range m.Parts {
		b, err := json.Marshal(p)
		if err != nil {
			return nil, err
		}
		raw.Parts = append(raw.Parts, b)
	}
	return json.Marshal(raw)
}

func (m *A2AMessage) UnmarshalJSON(data []byte) error {
	type Alias A2AMessage
	raw := &struct {
		RawParts []json.RawMessage `json:"parts"`
		*Alias
	}{Alias: (*Alias)(m)}
	if err := json.Unmarshal(data, raw); err != nil {
		return err
	}
	m.Parts = make([]Part, len(raw.RawParts))
	for i, rp := range raw.RawParts {
		var typeHint struct {
			TypeName string `json:"type"`
		}
		// 类型提示解析失败时回退到 TextPart
		if err := json.Unmarshal(rp, &typeHint); err != nil {
			var p TextPart
			m.Parts[i] = p
			continue
		}
		switch typeHint.TypeName {
		case "text":
			var p TextPart
			if err := json.Unmarshal(rp, &p); err != nil {
				p = TextPart{TypeField: "text"}
			}
			m.Parts[i] = p
		case "file":
			var p FilePart
			if err := json.Unmarshal(rp, &p); err != nil {
				p = FilePart{TypeField: "file"}
			}
			m.Parts[i] = p
		case "data":
			var p DataPart
			if err := json.Unmarshal(rp, &p); err != nil {
				p = DataPart{TypeField: "data"}
			}
			m.Parts[i] = p
		default:
			var p TextPart
			if err := json.Unmarshal(rp, &p); err != nil {
				p = TextPart{TypeField: "text"}
			}
			m.Parts[i] = p
		}
	}
	return nil
}

// ===== Artifact =====

type Artifact struct {
	ArtifactID string    `json:"artifact_id"`
	MimeType   string    `json:"mimetype"`
	Bytes      []byte    `json:"bytes,omitempty"`
	URI        string    `json:"uri,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// ExtractTextFromParts 从 Parts 列表中提取所有文本内容
func ExtractTextFromParts(parts []Part) string {
	var sb stringsBuilder
	for _, p := range parts {
		if tp, ok := p.(TextPart); ok {
			sb.WriteString(tp.Text)
		}
	}
	return sb.String()
}

type stringsBuilder struct{ data []byte }

func (b *stringsBuilder) WriteString(s string) int {
	b.data = append(b.data, s...)
	return len(s)
}

func (b *stringsBuilder) String() string { return string(b.data) }

func generateID(prefix string) string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%s-%s", prefix, hex.EncodeToString(b))
}
