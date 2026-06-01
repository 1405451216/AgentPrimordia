package debugger

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Visualizer struct {
}

func NewVisualizer() *Visualizer {
	return &Visualizer{}
}

type MemorySnapshot struct {
	TotalEpisodes int64         `json:"total_episodes"`
	TopSessions   []SessionInfo `json:"top_sessions"`
	RecentEvents  []RecentEvent `json:"recent_events"`
}

type SessionInfo struct {
	SessionID string `json:"session_id"`
	Count     int64  `json:"count"`
}

type RecentEvent struct {
	Time    string `json:"time"`
	Role    string `json:"role"`
	Content string `json:"content"`
}

func (v *Visualizer) RenderMemorySnapshot(snapshot *MemorySnapshot) string {
	var sb strings.Builder

	sb.WriteString("\n╔════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║                    MEMORY SNAPSHOT                          ║\n")
	sb.WriteString("╠════════════════════════════════════════════════════════════╣\n")
	sb.WriteString(fmt.Sprintf("║ Total Episodes: %-10d                                    ║\n", snapshot.TotalEpisodes))
	sb.WriteString("╠════════════════════════════════════════════════════════════╣\n")
	sb.WriteString("║ Top Sessions:                                               ║\n")

	for _, session := range snapshot.TopSessions {
		sb.WriteString(fmt.Sprintf("║   - %-30s (Count: %5d) ║\n", session.SessionID, session.Count))
	}

	sb.WriteString("╠════════════════════════════════════════════════════════════╣\n")
	sb.WriteString("║ Recent Events:                                              ║\n")

	for _, event := range snapshot.RecentEvents {
		sb.WriteString(fmt.Sprintf("║   [%s] %s: %s\n", event.Time, event.Role, truncateString(event.Content, 50)))
	}

	sb.WriteString("╚════════════════════════════════════════════════════════════╝\n")

	return sb.String()
}

func (v *Visualizer) RenderAsJSON(data interface{}) string {
	bytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error rendering JSON: %v", err)
	}
	return string(bytes)
}

func (v *Visualizer) RenderAgentLifecycle(states []LifecycleStep) string {
	var sb strings.Builder

	sb.WriteString("\n╔════════════════════════════════════════════════════════════╗\n")
	sb.WriteString("║               AGENT LIFECYCLE TRACE                         ║\n")
	sb.WriteString("╠════════════════════════════════════════════════════════════╣\n")

	for i, step := range states {
		sb.WriteString(fmt.Sprintf("║ Step %2d: %-25s %-18s ║\n", i+1, step.State, step.Timestamp.Format("15:04:05")))
		if step.Message != "" {
			sb.WriteString(fmt.Sprintf("║         Message: %s\n", truncateString(step.Message, 50)))
		}
	}

	sb.WriteString("╚════════════════════════════════════════════════════════════╝\n")

	return sb.String()
}

func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

type LifecycleStep struct {
	State     string
	Timestamp time.Time
	Message   string
}
