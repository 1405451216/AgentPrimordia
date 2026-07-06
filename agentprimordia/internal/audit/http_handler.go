package audit

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"
)

// HTTPHandler 审计日志 HTTP 查询接口。
//
// 支持的路由：
//   - GET /audit/events?actor=xxx&action=xxx&resource=xxx&start=2026-01-01T00:00:00Z&end=...&limit=100
//   - GET /audit/report?start=...&end=...
//   - GET /audit/health
//
// 使用方式：
//
//	logger, _ := audit.NewLogger(audit.LoggerConfig{Output: output})
//	handler := audit.NewHTTPHandler(logger)
//	http.Handle("/audit/", handler)
type HTTPHandler struct {
	logger *Logger
}

// NewHTTPHandler 创建 HTTP handler
func NewHTTPHandler(logger *Logger) *HTTPHandler {
	return &HTTPHandler{logger: logger}
}

// ServeHTTP 实现 http.Handler 接口
func (h *HTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	// 去除前缀路径，方便挂载到 /audit/
	path := r.URL.Path
	switch {
	case path == "/audit" || path == "/audit/" || path == "/audit/health":
		h.handleHealth(w, r)
	case path == "/audit/events" || hasPrefix(path, "/audit/events/"):
		h.handleQuery(w, r)
	case path == "/audit/report" || hasPrefix(path, "/audit/report/"):
		h.handleReport(w, r)
	default:
		writeJSONError(w, http.StatusNotFound, "not found")
	}
}

// handleHealth 健康检查
func (h *HTTPHandler) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleQuery 事件查询
func (h *HTTPHandler) handleQuery(w http.ResponseWriter, r *http.Request) {
	filter, err := parseQueryFilter(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	events, err := h.logger.Query(r.Context(), filter)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "query failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"count":  len(events),
		"events": events,
	})
}

// handleReport 报告生成
func (h *HTTPHandler) handleReport(w http.ResponseWriter, r *http.Request) {
	start, end, err := parseTimeRange(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	// 若指定了 template，使用模板化报告
	if tplStr := r.URL.Query().Get("template"); tplStr != "" {
		tpl := ReportTemplate(tplStr)
		cfg := ReportConfig{
			Template: tpl,
			Start:    start,
			End:      end,
			Actors:   r.URL.Query()["actor"], // 支持多个 ?actor=
		}
		report, err := h.logger.GenerateComplianceReport(r.Context(), cfg)
		if err != nil {
			if errors.Is(err, ErrInvalidTemplate) || errors.Is(err, ErrMissingPeriod) {
				writeJSONError(w, http.StatusBadRequest, err.Error())
				return
			}
			writeJSONError(w, http.StatusInternalServerError, "generate report failed: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, report)
		return
	}
	report, err := h.logger.GenerateReport(r.Context(), start, end)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "generate report failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

// parseQueryFilter 解析查询过滤器
func parseQueryFilter(r *http.Request) (QueryFilter, error) {
	q := r.URL.Query()
	f := QueryFilter{
		Actor:    q.Get("actor"),
		Action:   q.Get("action"),
		Resource: q.Get("resource"),
	}
	if start, err := parseTimeParam(q.Get("start")); err != nil {
		return f, err
	} else {
		f.Start = start
	}
	if end, err := parseTimeParam(q.Get("end")); err != nil {
		return f, err
	} else {
		f.End = end
	}
	if limitStr := q.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return f, &ParseError{"limit must be an integer"}
		}
		if limit < 0 {
			return f, &ParseError{"limit must be non-negative"}
		}
		f.Limit = limit
	}
	return f, nil
}

// parseTimeRange 解析时间范围参数
func parseTimeRange(r *http.Request) (time.Time, time.Time, error) {
	q := r.URL.Query()
	startStr := q.Get("start")
	endStr := q.Get("end")
	if startStr == "" || endStr == "" {
		return time.Time{}, time.Time{}, &ParseError{"start and end query parameters are required"}
	}
	start, err := parseTimeParam(startStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	end, err := parseTimeParam(endStr)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return start, end, nil
}

// parseTimeParam 解析时间字符串（支持 RFC3339 和 Unix 时间戳）
func parseTimeParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	// 尝试解析 RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// 回退到 Unix 时间戳
	if ts, err := strconv.ParseInt(s, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}
	return time.Time{}, &ParseError{"invalid time format: " + s + " (expected RFC3339 or unix timestamp)"}
}

// ParseError 解析错误
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return e.Msg }

// hasPrefix 字符串前缀检查（避免 strings 包导入）
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// writeJSON 写入 JSON 响应
func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// writeJSONError 写入 JSON 错误响应
func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// 编译期断言：确保 HTTPHandler 实现了 http.Handler 接口
var _ http.Handler = (*HTTPHandler)(nil)

// 引用 context 以避免导入未使用的警告
var _ = context.Background
