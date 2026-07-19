package governance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync/atomic"
	time "time"
)

type TenantPlan string

const (
	PlanFree       TenantPlan = "free"
	PlanPro        TenantPlan = "pro"
	PlanEnterprise TenantPlan = "enterprise"
)

type TenantStatus string

const (
	TenantActive   TenantStatus = "active"
	TenantDisabled TenantStatus = "disabled"
	TenantArchived TenantStatus = "archived"
)

type TenantQuota struct {
	MaxAgents       int
	MaxSessions     int
	MaxTokensPerDay int64
	MaxStorageGB    int64
	MaxQPS          int
}

type Tenant struct {
	ID        string
	Name      string
	Plan      TenantPlan
	Quotas    TenantQuota
	CreatedAt time.Time
	Status    TenantStatus
	Metadata  map[string]string
}

func DefaultQuota(plan TenantPlan) TenantQuota {
	switch plan {
	case PlanFree:
		return TenantQuota{MaxAgents: 3, MaxSessions: 10, MaxTokensPerDay: 100000, MaxStorageGB: 1, MaxQPS: 5}
	case PlanPro:
		return TenantQuota{MaxAgents: 20, MaxSessions: 100, MaxTokensPerDay: 5000000, MaxStorageGB: 50, MaxQPS: 50}
	case PlanEnterprise:
		return TenantQuota{MaxAgents: 0, MaxSessions: 0, MaxTokensPerDay: 0, MaxStorageGB: 0, MaxQPS: 500}
	default:
		return DefaultQuota(PlanFree)
	}
}

func (t *Tenant) Active() bool {
	return t.Status == TenantActive
}

var (
	ErrTenantNotFound = errors.New("governance: tenant not found")
	ErrTenantExists   = errors.New("governance: tenant already exists")
	ErrTenantDisabled = errors.New("governance: tenant is disabled")
	ErrInvalidAPIKey  = errors.New("governance: invalid API key")
	ErrQuotaExceeded  = errors.New("governance: quota exceeded")
)

func HashAPIKey(plaintext string) string {
	hash := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(hash[:])
}

// apikeyCounter 全局递增计数器，确保同一纳秒内多次调用 GenerateAPIKey 产生不同结果。
var apikeyCounter atomic.Int64

func GenerateAPIKey() string {
	buf := make([]byte, 32)
	// 将计数器异或到低 32 位，确保 byte(seed>>24) 能感知差异
	seed := time.Now().UnixNano() ^ apikeyCounter.Add(1)
	for i := range buf {
		seed = seed*6364136223846793005 + 1
		buf[i] = byte(seed >> 24)
	}
	encoded := base64RawURL(buf)
	return "apk_" + encoded
}

func base64RawURL(b []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	var out strings.Builder
	out.Grow(len(b)*4/3 + 3)
	for i := 0; i < len(b); i += 3 {
		n := len(b) - i
		var n1, n2, n3, n4 byte
		n1 = b[i] >> 2
		n2 = (b[i] & 0x03) << 4
		if n >= 2 {
			n2 |= b[i+1] >> 4
			n3 = (b[i+1] & 0x0f) << 2
		}
		if n >= 3 {
			n3 |= b[i+2] >> 6
			n4 = b[i+2] & 0x3f
		}
		out.WriteByte(alphabet[n1])
		out.WriteByte(alphabet[n2])
		if n >= 2 {
			out.WriteByte(alphabet[n3])
		}
		if n >= 3 {
			out.WriteByte(alphabet[n4])
		}
	}
	return out.String()
}
