package a2a

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc/credentials"
)

// TLSConfig 定义 gRPC 双向 TLS 配置。
type TLSConfig struct {
	// CertFile 服务端/客户端证书文件路径（PEM 格式）
	CertFile string
	// KeyFile 服务端/客户端私钥文件路径（PEM 格式）
	KeyFile string
	// CAFile CA 证书文件路径（PEM 格式），用于验证对端
	CAFile string
	// ServerName Override server name for certificate verification (optional)
	ServerName string
	// InsecureSkipVerify 跳过证书验证（仅测试用，生产环境必须为 false）
	InsecureSkipVerify bool
	// AutoRotation 是否启用证书自动轮换
	AutoRotation bool
	// RotationInterval 证书轮换检查间隔（默认 24h）
	RotationInterval time.Duration
}

// tlsManager 管理 TLS 证书加载和自动轮换。
type tlsManager struct {
	config   TLSConfig
	creds    credentials.TransportCredentials
	mu       sync.RWMutex
	stopCh   chan struct{}
	lastLoad time.Time
	logger   *slog.Logger
}

// NewTLSManager 创建 TLS 管理器。
func NewTLSManager(config TLSConfig, logger *slog.Logger) (*tlsManager, error) {
	if config.CertFile == "" || config.KeyFile == "" {
		return nil, fmt.Errorf("mtls: cert file and key file are required")
	}
	if config.CAFile == "" {
		return nil, fmt.Errorf("mtls: CA file is required for mutual TLS")
	}
	if config.RotationInterval <= 0 {
		config.RotationInterval = 24 * time.Hour
	}
	if logger == nil {
		logger = slog.Default()
	}

	tm := &tlsManager{
		config: config,
		stopCh: make(chan struct{}),
		logger: logger,
	}

	if err := tm.loadCredentials(); err != nil {
		return nil, fmt.Errorf("mtls: initial load: %w", err)
	}

	if config.AutoRotation {
		go tm.rotationLoop()
	}

	return tm, nil
}

// Credentials 返回当前 TLS 传输凭证。
func (tm *tlsManager) Credentials() credentials.TransportCredentials {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.creds
}

// Close 停止证书轮换循环。
func (tm *tlsManager) Close() {
	close(tm.stopCh)
}

// loadCredentials 从磁盘加载证书并构建 TransportCredentials。
func (tm *tlsManager) loadCredentials() error {
	cert, err := tls.LoadX509KeyPair(tm.config.CertFile, tm.config.KeyFile)
	if err != nil {
		return fmt.Errorf("load cert/key pair: %w", err)
	}

	caPool := x509.NewCertPool()
	caData, err := os.ReadFile(tm.config.CAFile)
	if err != nil {
		return fmt.Errorf("read CA file: %w", err)
	}
	if !caPool.AppendCertsFromPEM(caData) {
		return fmt.Errorf("failed to parse CA certificate")
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS12,
	}

	if tm.config.ServerName != "" {
		tlsConfig.ServerName = tm.config.ServerName
	}
	if tm.config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
		tm.logger.Warn("mTLS: InsecureSkipVerify is enabled, this should only be used in tests")
	}

	creds := credentials.NewTLS(tlsConfig)

	tm.mu.Lock()
	tm.creds = creds
	tm.lastLoad = time.Now()
	tm.mu.Unlock()

	return nil
}

// rotationLoop 定期检查证书文件是否变更，自动重新加载。
func (tm *tlsManager) rotationLoop() {
	ticker := time.NewTicker(tm.config.RotationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-tm.stopCh:
			return
		case <-ticker.C:
			if tm.shouldRotate() {
				if err := tm.loadCredentials(); err != nil {
					tm.logger.Error("mTLS 证书轮换失败", "error", err)
				} else {
					tm.logger.Info("mTLS 证书已轮换", "time", tm.lastLoad)
				}
			}
		}
	}
}

// shouldRotate 检查证书文件是否已修改。
func (tm *tlsManager) shouldRotate() bool {
	for _, f := range []string{tm.config.CertFile, tm.config.KeyFile, tm.config.CAFile} {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		if info.ModTime().After(tm.lastLoad) {
			return true
		}
	}
	return false
}

// ServerTLS 返回 gRPC ServerOption 用于启用 TLS。
func (tm *tlsManager) ServerTLS() credentials.TransportCredentials {
	return tm.Credentials()
}

// ClientTLS 返回 gRPC DialOption 用于启用 TLS。
func (tm *tlsManager) ClientTLS() credentials.TransportCredentials {
	return tm.Credentials()
}

// ===== 便捷函数 =====

// ServerTLSCredentials 从 TLSConfig 创建服务端 TLS 凭证（无自动轮换）。
func ServerTLSCredentials(config TLSConfig) (credentials.TransportCredentials, error) {
	if config.CertFile == "" || config.KeyFile == "" {
		return nil, fmt.Errorf("mtls: cert file and key file are required")
	}

	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert/key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if config.CAFile != "" {
		caPool := x509.NewCertPool()
		caData, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.ClientCAs = caPool
		// 要求客户端提供证书（mTLS）
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	}

	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}
	if config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return credentials.NewTLS(tlsConfig), nil
}

// ClientTLSCredentials 从 TLSConfig 创建客户端 TLS 凭证（无自动轮换）。
func ClientTLSCredentials(config TLSConfig) (credentials.TransportCredentials, error) {
	if config.CertFile == "" || config.KeyFile == "" {
		return nil, fmt.Errorf("mtls: cert file and key file are required")
	}

	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert/key pair: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	}

	if config.CAFile != "" {
		caPool := x509.NewCertPool()
		caData, err := os.ReadFile(config.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA file: %w", err)
		}
		if !caPool.AppendCertsFromPEM(caData) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caPool
	}

	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}
	if config.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	return credentials.NewTLS(tlsConfig), nil
}

// 编译期检查 _ = context.Background
var _ = context.Background
