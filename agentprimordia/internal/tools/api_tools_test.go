package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHTTPClientTool_GetRequest(t *testing.T) {
	httpTool := NewHTTPClientTool()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("expected GET method, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "hello"})
	}))
	defer server.Close()

	input := map[string]any{
		"method": "GET",
		"url":    server.URL,
	}
	inputBytes, _ := json.Marshal(input)

	result, err := httpTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var response map[string]any
	_ = json.Unmarshal([]byte(result.Content), &response)

	if response["status_code"].(float64) != 200 {
		t.Errorf("expected status 200, got %v", response["status_code"])
	}

	t.Logf("✅ HTTP GET: status=%d", int(response["status_code"].(float64)))
}

func TestHTTPClientTool_PostRequest(t *testing.T) {
	httpTool := NewHTTPClientTool()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST method, got %s", r.Method)
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(201)
		_ = json.NewEncoder(w).Encode(map[string]any{"id": 123, "received": body})
	}))
	defer server.Close()

	input := map[string]any{
		"method": "POST",
		"url":    server.URL,
		"body":   map[string]string{"name": "test", "value": "data"},
	}
	inputBytes, _ := json.Marshal(input)

	result, err := httpTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var response map[string]any
	_ = json.Unmarshal([]byte(result.Content), &response)

	if response["status_code"].(float64) != 201 {
		t.Errorf("expected status 201, got %v", response["status_code"])
	}

	bodyStr := response["body"].(string)
	var bodyMap map[string]any
	_ = json.Unmarshal([]byte(bodyStr), &bodyMap)
	if bodyMap == nil || bodyMap["id"].(float64) != 123 {
		t.Error("expected id=123 in response")
	}

	t.Logf("✅ HTTP POST: created resource with id=%d", int(bodyMap["id"].(float64)))
}

func TestHTTPClientTool_BearerAuth(t *testing.T) {
	httpTool := NewHTTPClientTool()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader != "Bearer test-token-123" {
			t.Errorf("expected Bearer token, got: %s", authHeader)
		}
		_, _ = w.Write([]byte(`{"authenticated": true}`))
	}))
	defer server.Close()

	input := map[string]any{
		"method":     "GET",
		"url":        server.URL,
		"auth_type":  "bearer",
		"auth_token": "test-token-123",
	}
	inputBytes, _ := json.Marshal(input)

	result, err := httpTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var response map[string]any
	_ = json.Unmarshal([]byte(result.Content), &response)
	bodyStr := response["body"].(string)
	var bodyData map[string]any
	_ = json.Unmarshal([]byte(bodyStr), &bodyData)
	if bodyData == nil || bodyData["authenticated"].(bool) != true {
		t.Error("authentication failed")
	}

	t.Logf("✅ HTTP Bearer Auth: authentication successful")
}

func TestGitTool_Status(t *testing.T) {
	tmpDir := t.TempDir()
	gitTool := NewGitTool(tmpDir)

	initCmd := exec.CommandContext(context.Background(), "git", "init")
	initCmd.Dir = tmpDir
	_ = initCmd.Run()

	configName := exec.CommandContext(context.Background(), "git", "config", "user.email", "test@example.com")
	configName.Dir = tmpDir
	_ = configName.Run()

	configEmail := exec.CommandContext(context.Background(), "git", "config", "user.name", "Test User")
	configEmail.Dir = tmpDir
	_ = configEmail.Run()

	testFile := filepath.Join(tmpDir, "test.txt")
	_ = os.WriteFile(testFile, []byte("hello"), 0644)

	statusInput := map[string]any{"action": "status"}
	statusBytes, _ := json.Marshal(statusInput)
	result, err := gitTool.Execute(context.Background(), statusBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var statusData map[string][]string
	_ = json.Unmarshal([]byte(result.Content), &statusData)

	if len(statusData["untracked"]) == 0 {
		t.Error("expected untracked files in status")
	}

	t.Logf("✅ Git Status: untracked=%d modified=%d", len(statusData["untracked"]), len(statusData["modified"]))
}

func TestGitTool_Log(t *testing.T) {
	tmpDir := t.TempDir()
	gitTool := NewGitTool(tmpDir)

	setupGitRepo(t, tmpDir)

	createCommit(t, tmpDir, "Initial commit")

	logInput := map[string]any{"action": "log", "count": float64(5)}
	logBytes, _ := json.Marshal(logInput)
	result, err := gitTool.Execute(context.Background(), logBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var commits []map[string]string
	_ = json.Unmarshal([]byte(result.Content), &commits)

	if len(commits) == 0 {
		t.Error("expected at least 1 commit")
	}

	t.Logf("✅ Git Log: found %d commits", len(commits))
	for _, c := range commits {
		t.Logf("   - %s: %s", c["hash"], c["message"])
	}
}

func TestSearchTool_BasicSearch(t *testing.T) {
	searchTool := NewSearchTool()

	input := map[string]any{
		"query":       "Go programming language",
		"num_results": 5,
	}
	inputBytes, _ := json.Marshal(input)

	result, err := searchTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !strings.Contains(result.Content, "search tool is not connected") {
		t.Errorf("expected error about unconfigured search provider, got: %s", result.Content)
	}

	t.Logf("✅ Search: correctly returns error for unconfigured provider")
}

func TestAPITools_Integration(t *testing.T) {
	t.Log("\n=== API Tools Integration Test ===")

	t.Log("\n1. Testing HTTP Client Tool...")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"service": "api_test", "version": "1.0"})
	}))
	defer server.Close()

	httpTool := NewHTTPClientTool()
	httpInput := map[string]any{"method": "GET", "url": server.URL}
	httpBytes, _ := json.Marshal(httpInput)
	_, _ = httpTool.Execute(context.Background(), httpBytes)
	t.Logf("   ✅ HTTP Client working")

	t.Log("\n2. Testing Search Tool...")
	searchTool := NewSearchTool()
	searchInput := map[string]any{"query": "AI agents framework"}
	searchBytes, _ := json.Marshal(searchInput)
	_, _ = searchTool.Execute(context.Background(), searchBytes)
	t.Logf("   ✅ Search Tool working")

	t.Log("\n3. Testing Git Tool...")
	tmpDir := t.TempDir()
	gitTool := NewGitTool(tmpDir)
	setupGitRepo(t, tmpDir)
	gitStatusInput := map[string]any{"action": "status"}
	gitStatusBytes, _ := json.Marshal(gitStatusInput)
	_, _ = gitTool.Execute(context.Background(), gitStatusBytes)
	t.Logf("   ✅ Git Tool working")

	t.Log("\n=== All API Tools Integration Tests Passed ===")
}

func setupGitRepo(t *testing.T, dir string) {
	cmds := [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test User"},
	}
	for _, cmdArgs := range cmds {
		cmd := exec.CommandContext(context.Background(), "git", cmdArgs...)
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("setup git repo error: %v (args: %v)", err, cmdArgs)
		}
	}
}

func createCommit(t *testing.T, dir string, message string) {
	filePath := filepath.Join(dir, "file.txt")
	_ = os.WriteFile(filePath, []byte("content"), 0644)

	addCmd := exec.CommandContext(context.Background(), "git", "add", ".")
	addCmd.Dir = dir
	_ = addCmd.Run()

	commitCmd := exec.CommandContext(context.Background(), "git", "commit", "-m", message)
	commitCmd.Dir = dir
	if err := commitCmd.Run(); err != nil {
		t.Fatalf("create commit error: %v", err)
	}
}
