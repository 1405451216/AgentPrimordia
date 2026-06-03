package debugger

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

type DebugServer struct {
	addr      string
	mu        sync.RWMutex
	events    []DebugEvent
	snapshots []MemorySnapshot
	mux       *http.ServeMux
	server    *http.Server
}

type DebugEvent struct {
	Type      string
	Message   string
	Timestamp string
}

func NewDebugServer(addr string) *DebugServer {
	d := &DebugServer{
		addr:      addr,
		events:    make([]DebugEvent, 0, 100),
		snapshots: make([]MemorySnapshot, 0, 10),
		mux:       http.NewServeMux(),
	}

	d.mux.HandleFunc("/", d.handleIndex)
	d.mux.HandleFunc("/api/events", d.handleEvents)
	d.mux.HandleFunc("/api/snapshots", d.handleSnapshots)

	return d
}

// Handler 返回实例级 HTTP 路由处理器，可用于 httptest.NewServer
func (d *DebugServer) Handler() http.Handler {
	return d.mux
}

func (d *DebugServer) Start() error {
	d.server = &http.Server{
		Addr:    d.addr,
		Handler: d.mux,
	}

	slog.Info("Debug server listening", "addr", d.addr)
	go func() {
		if err := d.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Debug server error", "error", err)
		}
	}()
	return nil
}

func (d *DebugServer) AddEvent(eventType, message string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.events) >= 100 {
		d.events = d.events[1:]
	}

	d.events = append(d.events, DebugEvent{
		Type:      eventType,
		Message:   message,
		Timestamp: time.Now().Format("2006-01-02 15:04:05"),
	})
}

func (d *DebugServer) AddSnapshot(snapshot MemorySnapshot) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.snapshots) >= 10 {
		d.snapshots = d.snapshots[1:]
	}
	d.snapshots = append(d.snapshots, snapshot)
}

func (d *DebugServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<title>Agent Debugger</title>
	<style>
		body {
			font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
			margin: 0;
			padding: 20px;
			background: #f5f5f5;
		}
		.container {
			max-width: 1200px;
			margin: 0 auto;
		}
		h1 {
			color: #333;
			margin-bottom: 30px;
		}
		.card {
			background: white;
			border-radius: 8px;
			padding: 20px;
			margin-bottom: 20px;
			box-shadow: 0 2px 10px rgba(0,0,0,0.1);
		}
		table {
			width: 100%;
			border-collapse: collapse;
		}
		th, td {
			padding: 12px;
			text-align: left;
			border-bottom: 1px solid #eee;
		}
		th {
			background: #f9f9f9;
			color: #666;
		}
		tr:hover {
			background: #f9f9f9;
		}
	</style>
</head>
<body>
	<div class="container">
		<h1>🔧 Agent Debugger</h1>
		
		<div class="card">
			<h2>Events</h2>
			<div id="events-container">Loading...</div>
		</div>

		<div class="card">
			<h2>Memory Snapshots</h2>
			<div id="snapshots-container">Loading...</div>
		</div>
	</div>

	<script>
		async function loadEvents() {
			const res = await fetch('/api/events');
			const events = await res.json();
			const container = document.getElementById('events-container');
			container.innerHTML = '<table>' + 
				'<thead><tr><th>Timestamp</th><th>Type</th><th>Message</th></tr></thead>' + 
				'<tbody>' + 
				events.map(e => '<tr><td>' + e.Timestamp + '</td><td>' + e.Type + '</td><td>' + e.Message + '</td></tr>').join('') + 
				'</tbody></table>';
		}

		async function loadSnapshots() {
			const res = await fetch('/api/snapshots');
			const snapshots = await res.json();
			const container = document.getElementById('snapshots-container');
			container.innerHTML = '<table>' + 
				'<thead><tr><th>Total Episodes</th><th>Top Sessions</th><th>Recent Events</th></tr></thead>' + 
				'<tbody>' + 
				snapshots.map(s => '<tr><td>' + s.TotalEpisodes + '</td><td>' + 
					(s.TopSessions ? s.TopSessions.map(t => t.SessionID + '(' + t.Count + ')').join(', ') : 'None') + 
					'</td><td>' + s.RecentEvents.length + ' events</td></tr>').join('') + 
				'</tbody></table>';
		}

		loadEvents();
		loadSnapshots();
		setInterval(() => {
			loadEvents();
			loadSnapshots();
		}, 3000);
	</script>
</body>
</html>
`))
}

func (d *DebugServer) handleEvents(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	events := make([]DebugEvent, len(d.events))
	copy(events, d.events)
	d.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(events)
}

func (d *DebugServer) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	d.mu.RLock()
	snapshots := make([]MemorySnapshot, len(d.snapshots))
	copy(snapshots, d.snapshots)
	d.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshots)
}
