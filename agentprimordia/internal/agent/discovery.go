package agent

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"sync"
	"time"
)

// Discovery Agent 发现协议接口
type Discovery interface {
	// Register 注册本地 Agent
	Register(ctx context.Context, info *AgentInfo) error
	// Unregister 注销 Agent
	Unregister(ctx context.Context, agentID string) error
	// Discover 发现指定 Agent
	Discover(ctx context.Context, agentID string) (*AgentInfo, error)
	// ListAgents 列出所有已注册 Agent
	ListAgents(ctx context.Context) ([]*AgentInfo, error)
	// Heartbeat 心跳（保持注册活跃）
	Heartbeat(ctx context.Context, agentID string) error
	// Close 关闭发现服务
	Close() error
}

// AgentInfo Agent 注册信息
type AgentInfo struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Address      string            `json:"address"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
	LastSeen     time.Time         `json:"last_seen"`
}

// copy 返回 AgentInfo 的深拷贝
func (a *AgentInfo) copy() *AgentInfo {
	cp := *a
	if a.Capabilities != nil {
		cp.Capabilities = make([]string, len(a.Capabilities))
		copy(cp.Capabilities, a.Capabilities)
	}
	if a.Metadata != nil {
		cp.Metadata = make(map[string]string, len(a.Metadata))
		maps.Copy(cp.Metadata, a.Metadata)
	}
	return &cp
}

// ===== LocalDiscovery 进程内实现 =====

// LocalDiscovery 进程内 Agent 发现服务
type LocalDiscovery struct {
	agents map[string]*AgentInfo
	mu     sync.RWMutex
	logger *slog.Logger
}

// NewLocalDiscovery 创建本地发现服务
func NewLocalDiscovery() *LocalDiscovery {
	return &LocalDiscovery{
		agents: make(map[string]*AgentInfo),
		logger: slog.Default(),
	}
}

// Register 注册本地 Agent
func (d *LocalDiscovery) Register(ctx context.Context, info *AgentInfo) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	stored := info.copy()
	stored.LastSeen = time.Now()
	d.agents[info.ID] = stored

	d.logger.Info("Agent 注册到发现服务", "id", info.ID, "name", info.Name)
	return nil
}

// Unregister 注销 Agent
func (d *LocalDiscovery) Unregister(ctx context.Context, agentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	delete(d.agents, agentID)
	d.logger.Info("Agent 从发现服务注销", "id", agentID)
	return nil
}

// Discover 发现指定 Agent
func (d *LocalDiscovery) Discover(ctx context.Context, agentID string) (*AgentInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	info, ok := d.agents[agentID]
	if !ok {
		return nil, fmt.Errorf("agent %q not found in discovery", agentID)
	}
	return info.copy(), nil
}

// ListAgents 列出所有已注册 Agent
func (d *LocalDiscovery) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	result := make([]*AgentInfo, 0, len(d.agents))
	for _, info := range d.agents {
		result = append(result, info.copy())
	}
	return result, nil
}

// Heartbeat 心跳（保持注册活跃）
func (d *LocalDiscovery) Heartbeat(ctx context.Context, agentID string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	info, ok := d.agents[agentID]
	if !ok {
		return fmt.Errorf("agent %q not found in discovery", agentID)
	}
	info.LastSeen = time.Now()
	return nil
}

// Close 关闭发现服务（本地实现为空操作）
func (d *LocalDiscovery) Close() error {
	return nil
}

// ===== HTTPDiscovery HTTP 客户端实现 =====

// HTTPDiscovery 基于 HTTP 的远程 Agent 发现客户端
type HTTPDiscovery struct {
	baseURL    string
	client     *http.Client
	localCache map[string]*AgentInfo
	mu         sync.RWMutex
	logger     *slog.Logger
}

// NewHTTPDiscovery 创建 HTTP 发现客户端
func NewHTTPDiscovery(registryURL string) *HTTPDiscovery {
	return &HTTPDiscovery{
		baseURL:    registryURL,
		client:     &http.Client{Timeout: 30 * time.Second},
		localCache: make(map[string]*AgentInfo),
		logger:     slog.Default(),
	}
}

// Register 注册 Agent 到远程注册中心
func (d *HTTPDiscovery) Register(ctx context.Context, info *AgentInfo) error {
	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal agent info failed: %w", err)
	}

	url := d.baseURL + "/api/discovery/register"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("register request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("register returned status %d: %s", resp.StatusCode, string(body))
	}

	d.mu.Lock()
	d.localCache[info.ID] = info.copy()
	d.mu.Unlock()

	return nil
}

// Unregister 从远程注册中心注销 Agent
func (d *HTTPDiscovery) Unregister(ctx context.Context, agentID string) error {
	url := fmt.Sprintf("%s/api/discovery/%s", d.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("unregister request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unregister returned status %d: %s", resp.StatusCode, string(body))
	}

	d.mu.Lock()
	delete(d.localCache, agentID)
	d.mu.Unlock()

	return nil
}

// Discover 从远程注册中心发现指定 Agent
func (d *HTTPDiscovery) Discover(ctx context.Context, agentID string) (*AgentInfo, error) {
	url := fmt.Sprintf("%s/api/discovery/%s", d.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("discover request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("discover returned status %d: %s", resp.StatusCode, string(body))
	}

	var info AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	d.mu.Lock()
	d.localCache[info.ID] = info.copy()
	d.mu.Unlock()

	return &info, nil
}

// ListAgents 从远程注册中心列出所有 Agent
func (d *HTTPDiscovery) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	url := d.baseURL + "/api/discovery/agents"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("list agents request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list agents returned status %d: %s", resp.StatusCode, string(body))
	}

	var agents []*AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agents); err != nil {
		return nil, fmt.Errorf("decode response failed: %w", err)
	}

	d.mu.Lock()
	d.localCache = make(map[string]*AgentInfo, len(agents))
	for _, a := range agents {
		d.localCache[a.ID] = a.copy()
	}
	d.mu.Unlock()

	return agents, nil
}

// Heartbeat 向远程注册中心发送心跳
func (d *HTTPDiscovery) Heartbeat(ctx context.Context, agentID string) error {
	url := fmt.Sprintf("%s/api/discovery/%s/heartbeat", d.baseURL, agentID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("heartbeat request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("heartbeat returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Close 关闭发现客户端，清空本地缓存
func (d *HTTPDiscovery) Close() error {
	d.mu.Lock()
	d.localCache = make(map[string]*AgentInfo)
	d.mu.Unlock()
	return nil
}

// ===== DiscoveryServer HTTP 服务端 =====

const discoveryShutdownTimeout = 5 * time.Second

// DiscoveryServer Agent 发现 HTTP 服务端
type DiscoveryServer struct {
	discovery *LocalDiscovery
	server    *http.Server
	addr      string
	apiKey    string
	mu        sync.RWMutex
	started   bool
	logger    *slog.Logger
}

// NewDiscoveryServer 创建发现服务 HTTP 服务端
func NewDiscoveryServer(discovery *LocalDiscovery) *DiscoveryServer {
	return &DiscoveryServer{
		discovery: discovery,
		logger:    slog.Default(),
	}
}

// WithAPIKey 设置 API Key 认证密钥，设置后所有写操作需携带 Authorization: Bearer <key> 头
func (s *DiscoveryServer) WithAPIKey(key string) *DiscoveryServer {
	s.apiKey = key
	return s
}

// checkAuth 检查请求认证，如果配置了 APIKey 则验证 Bearer Token
func (s *DiscoveryServer) checkAuth(r *http.Request) bool {
	if s.apiKey == "" {
		return true
	}
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:] == s.apiKey
	}
	return false
}

// authMiddleware 认证中间件，保护写操作端点
func (s *DiscoveryServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.checkAuth(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// handler 返回 HTTP 路由处理器
func (s *DiscoveryServer) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/discovery/register", s.authMiddleware(s.handleRegister))
	mux.HandleFunc("DELETE /api/discovery/{id}", s.authMiddleware(s.handleUnregister))
	mux.HandleFunc("GET /api/discovery/agents", s.handleListAgents)
	mux.HandleFunc("GET /api/discovery/{id}", s.handleDiscover)
	mux.HandleFunc("POST /api/discovery/{id}/heartbeat", s.authMiddleware(s.handleHeartbeat))
	return mux
}

// Start 在指定地址启动发现服务
func (s *DiscoveryServer) Start(addr string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.started {
		return fmt.Errorf("discovery server already started on %s", s.addr)
	}

	s.server = &http.Server{
		Handler: s.handler(),
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s failed: %w", addr, err)
	}

	s.addr = ln.Addr().String()
	s.started = true

	go func() {
		if err := s.server.Serve(ln); err != nil && err != http.ErrServerClosed {
			s.logger.Error("Discovery server error", "error", err)
		}
	}()

	s.logger.Info("Discovery server 已启动", "addr", s.addr)
	return nil
}

// Addr 返回实际监听地址
func (s *DiscoveryServer) Addr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.addr
}

// Close 优雅关闭发现服务
func (s *DiscoveryServer) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.started {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), discoveryShutdownTimeout)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown discovery server failed: %w", err)
	}

	s.started = false
	s.logger.Info("Discovery server 已关闭", "addr", s.addr)
	return nil
}

// handleRegister 处理注册请求
func (s *DiscoveryServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 限制请求体 1MB

	var info AgentInfo
	if err := json.NewDecoder(r.Body).Decode(&info); err != nil {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body", "detail": err.Error()})
		return
	}

	if info.ID == "" || len(info.ID) > 256 {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "agent id must be between 1 and 256 characters"})
		return
	}
	if info.Name == "" || len(info.Name) > 256 {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "agent name must be between 1 and 256 characters"})
		return
	}
	if len(info.Address) > 1024 {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "agent address must not exceed 1024 characters"})
		return
	}

	if err := s.discovery.Register(r.Context(), &info); err != nil {
		writeDiscoveryJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

type Authenticator interface {
	Authenticate(token string) (*AgentIdentity, error)
	GenerateToken(identity *AgentIdentity) (string, error)
}

type AgentIdentity struct {
	ID       string
	Name     string
	Roles    []string
	Metadata map[string]string
}

type TokenAuthenticator struct {
	secret []byte
}

func NewTokenAuthenticator(secret string) *TokenAuthenticator {
	return &TokenAuthenticator{
		secret: []byte(secret),
	}
}

func (a *TokenAuthenticator) Authenticate(token string) (*AgentIdentity, error) {
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}

	parts := splitToken(token)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	payload, err := base64.URLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload: %w", err)
	}

	signature, err := base64.URLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token signature: %w", err)
	}

	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(signature, expectedSig) {
		return nil, fmt.Errorf("invalid token signature")
	}

	var identity AgentIdentity
	if err := json.Unmarshal(payload, &identity); err != nil {
		return nil, fmt.Errorf("invalid token identity: %w", err)
	}

	return &identity, nil
}

func (a *TokenAuthenticator) GenerateToken(identity *AgentIdentity) (string, error) {
	payload, err := json.Marshal(identity)
	if err != nil {
		return "", fmt.Errorf("marshal identity failed: %w", err)
	}

	payloadEnc := base64.URLEncoding.EncodeToString(payload)

	mac := hmac.New(sha256.New, a.secret)
	mac.Write(payload)
	signature := mac.Sum(nil)
	sigEnc := base64.URLEncoding.EncodeToString(signature)

	timestamp := base64.URLEncoding.EncodeToString([]byte(time.Now().Format(time.RFC3339)))

	return payloadEnc + "." + sigEnc + "." + timestamp, nil
}

func splitToken(token string) []string {
	var parts []string
	start := 0
	for i, c := range token {
		if c == '.' {
			parts = append(parts, token[start:i])
			start = i + 1
		}
	}
	parts = append(parts, token[start:])
	return parts
}

type NoopAuthenticator struct{}

func NewNoopAuthenticator() *NoopAuthenticator {
	return &NoopAuthenticator{}
}

func (a *NoopAuthenticator) Authenticate(token string) (*AgentIdentity, error) {
	return &AgentIdentity{
		ID:    "anonymous",
		Name:  "anonymous",
		Roles: []string{"all"},
	}, nil
}

func (a *NoopAuthenticator) GenerateToken(identity *AgentIdentity) (string, error) {
	return "noop-token", nil
}

type AuthenticatedDiscovery struct {
	inner  *LocalDiscovery
	auth   Authenticator
	tokens map[string]*AgentIdentity
	mu     sync.RWMutex
}

func NewAuthenticatedDiscovery(inner *LocalDiscovery, auth Authenticator) *AuthenticatedDiscovery {
	return &AuthenticatedDiscovery{
		inner:  inner,
		auth:   auth,
		tokens: make(map[string]*AgentIdentity),
	}
}

func (d *AuthenticatedDiscovery) Register(ctx context.Context, info *AgentInfo, token string) error {
	identity, err := d.auth.Authenticate(token)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	d.mu.Lock()
	d.tokens[info.ID] = identity
	d.mu.Unlock()

	return d.inner.Register(ctx, info)
}

func (d *AuthenticatedDiscovery) Unregister(ctx context.Context, agentID string, token string) error {
	_, err := d.auth.Authenticate(token)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	d.mu.Lock()
	delete(d.tokens, agentID)
	d.mu.Unlock()

	return d.inner.Unregister(ctx, agentID)
}

func (d *AuthenticatedDiscovery) Discover(ctx context.Context, agentID string) (*AgentInfo, error) {
	return d.inner.Discover(ctx, agentID)
}

func (d *AuthenticatedDiscovery) ListAgents(ctx context.Context) ([]*AgentInfo, error) {
	return d.inner.ListAgents(ctx)
}

func (d *AuthenticatedDiscovery) ListAgentsByRole(ctx context.Context, role string) ([]*AgentInfo, error) {
	agents, err := d.inner.ListAgents(ctx)
	if err != nil {
		return nil, err
	}

	d.mu.RLock()
	defer d.mu.RUnlock()

	var filtered []*AgentInfo
	for _, a := range agents {
		if identity, ok := d.tokens[a.ID]; ok {
			for _, r := range identity.Roles {
				if r == role {
					filtered = append(filtered, a)
					break
				}
			}
		}
	}
	return filtered, nil
}

func (d *AuthenticatedDiscovery) Heartbeat(ctx context.Context, agentID string) error {
	return d.inner.Heartbeat(ctx, agentID)
}

func (d *AuthenticatedDiscovery) Close() error {
	return d.inner.Close()
}

// handleUnregister 处理注销请求
func (s *DiscoveryServer) handleUnregister(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent id"})
		return
	}

	if err := s.discovery.Unregister(r.Context(), id); err != nil {
		writeDiscoveryJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

// handleDiscover 处理发现请求
func (s *DiscoveryServer) handleDiscover(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent id"})
		return
	}

	info, err := s.discovery.Discover(r.Context(), id)
	if err != nil {
		writeDiscoveryJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// handleListAgents 处理列出 Agent 请求
func (s *DiscoveryServer) handleListAgents(w http.ResponseWriter, r *http.Request) {
	agents, err := s.discovery.ListAgents(r.Context())
	if err != nil {
		writeDiscoveryJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

// handleHeartbeat 处理心跳请求
func (s *DiscoveryServer) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		writeDiscoveryJSON(w, http.StatusBadRequest, map[string]string{"error": "missing agent id"})
		return
	}

	if err := s.discovery.Heartbeat(r.Context(), id); err != nil {
		writeDiscoveryJSON(w, http.StatusNotFound, map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
}

// writeDiscoveryJSON 写入 JSON 格式的错误响应，保持 API 响应格式一致
func writeDiscoveryJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
