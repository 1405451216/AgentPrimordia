package concurrency

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

var (
	ErrGlobalWriteConflict = errors.New("at most 1 task can have global write scope per batch")
	ErrScopeOverlap        = errors.New("task scopes overlap")
)

type fileLockEntry struct {
	mu       *sync.Mutex
	refCount int
}

type FileLockManager struct {
	mu    sync.Mutex
	locks map[string]*fileLockEntry
}

func NewFileLockManager() *FileLockManager {
	return &FileLockManager{
		locks: make(map[string]*fileLockEntry),
	}
}

func (m *FileLockManager) Acquire(path string) {
	m.mu.Lock()
	entry, exists := m.locks[path]
	if !exists {
		entry = &fileLockEntry{mu: &sync.Mutex{}}
		m.locks[path] = entry
	}
	entry.refCount++
	m.mu.Unlock()
	entry.mu.Lock()
}

func (m *FileLockManager) Release(path string) {
	m.mu.Lock()
	entry, exists := m.locks[path]
	if !exists {
		m.mu.Unlock()
		return
	}
	entry.refCount--
	if entry.refCount <= 0 {
		delete(m.locks, path)
	}
	m.mu.Unlock()
	entry.mu.Unlock()
}

func (m *FileLockManager) TryAcquire(path string) bool {
	m.mu.Lock()
	entry, exists := m.locks[path]
	if !exists {
		entry = &fileLockEntry{mu: &sync.Mutex{}}
		m.locks[path] = entry
	}
	entry.refCount++
	m.mu.Unlock()
	acquired := entry.mu.TryLock()
	if !acquired {
		m.mu.Lock()
		entry.refCount--
		if entry.refCount <= 0 {
			delete(m.locks, path)
		}
		m.mu.Unlock()
	}
	return acquired
}

func ValidateScopes(scopes [][]string) error {
	globalCount := 0
	for _, scope := range scopes {
		if isGlobalScope(scope) {
			globalCount++
		}
	}
	if globalCount > 1 {
		return fmt.Errorf("%w: found %d global scopes", ErrGlobalWriteConflict, globalCount)
	}

	for i := 0; i < len(scopes); i++ {
		if isGlobalScope(scopes[i]) || len(scopes[i]) == 0 {
			continue
		}
		for j := i + 1; j < len(scopes); j++ {
			if isGlobalScope(scopes[j]) || len(scopes[j]) == 0 {
				continue
			}
			if overlap := findScopeOverlap(scopes[i], scopes[j]); overlap != "" {
				return fmt.Errorf("%w: tasks %d and %d overlap at %s", ErrScopeOverlap, i+1, j+1, overlap)
			}
		}
	}
	return nil
}

// isGlobalScope 判断 scope 是否为显式全局权限（包含 "/" 路径）
func isGlobalScope(scope []string) bool {
	for _, s := range scope {
		cleaned := filepath.Clean(s)
		if cleaned == "/" || cleaned == `\` {
			return true
		}
	}
	return false
}

func findScopeOverlap(scopeA, scopeB []string) string {
	for _, a := range scopeA {
		cleanA := filepath.Clean(a)
		for _, b := range scopeB {
			cleanB := filepath.Clean(b)
			if cleanA == cleanB {
				return cleanA
			}
			if strings.HasPrefix(cleanA, cleanB+string(filepath.Separator)) {
				return fmt.Sprintf("%s (属于 %s)", cleanA, cleanB)
			}
			if strings.HasPrefix(cleanB, cleanA+string(filepath.Separator)) {
				return fmt.Sprintf("%s (属于 %s)", cleanB, cleanA)
			}
		}
	}
	return ""
}
