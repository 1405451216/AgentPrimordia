package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PluginInstaller 插件安装器
type PluginInstaller struct {
	installDir string
	market     MarketInterface
	loader     *PluginLoader
}

// NewPluginInstaller 创建插件安装器
func NewPluginInstaller(installDir string, market MarketInterface, loader *PluginLoader) (*PluginInstaller, error) {
	if installDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = "."
		}
		installDir = filepath.Join(home, ".agentprimordia", "installed")
	}

	if err := os.MkdirAll(installDir, 0755); err != nil {
		return nil, fmt.Errorf("tools: cannot create install dir: %w", err)
	}

	return &PluginInstaller{
		installDir: installDir,
		market:     market,
		loader:     loader,
	}, nil
}

// InstallFromMarket 从市场安装插件
func (pi *PluginInstaller) InstallFromMarket(ctx context.Context, name, version string) error {
	data, err := pi.market.Download(ctx, name, version)
	if err != nil {
		return fmt.Errorf("tools: download failed: %w", err)
	}

	installPath := filepath.Join(pi.installDir, name)
	if err := os.MkdirAll(installPath, 0755); err != nil {
		return fmt.Errorf("tools: create install path: %w", err)
	}

	pluginFile := filepath.Join(installPath, version+".tar.gz")
	if err := os.WriteFile(pluginFile, data, 0644); err != nil {
		return fmt.Errorf("tools: write plugin file: %w", err)
	}

	manifest, err := pi.market.GetManifest(name)
	if err != nil {
		return fmt.Errorf("tools: get manifest: %w", err)
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("tools: marshal manifest: %w", err)
	}
	manifestFile := filepath.Join(installPath, "manifest.json")
	if err := os.WriteFile(manifestFile, manifestData, 0644); err != nil {
		return fmt.Errorf("tools: write manifest: %w", err)
	}

	if manifest.Checksum != "" {
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != manifest.Checksum {
			os.RemoveAll(installPath)
			return fmt.Errorf("tools: checksum verification failed for %s@%s", name, version)
		}
	}

	return nil
}

// VerifyChecksum 校验已安装插件的完整性
func (pi *PluginInstaller) VerifyChecksum(name, version string) error {
	installPath := filepath.Join(pi.installDir, name)
	manifestFile := filepath.Join(installPath, "manifest.json")

	data, err := os.ReadFile(manifestFile)
	if err != nil {
		return fmt.Errorf("tools: read manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("tools: unmarshal manifest: %w", err)
	}

	if manifest.Checksum == "" {
		return nil
	}

	pluginFile := filepath.Join(installPath, version+".tar.gz")
	fileData, err := os.ReadFile(pluginFile)
	if err != nil {
		return fmt.Errorf("tools: read plugin file: %w", err)
	}

	sum := sha256.Sum256(fileData)
	if hex.EncodeToString(sum[:]) != manifest.Checksum {
		return fmt.Errorf("tools: checksum mismatch for %s@%s", name, version)
	}

	return nil
}

// Uninstall 卸载已安装的插件
func (pi *PluginInstaller) Uninstall(name string) error {
	installPath := filepath.Join(pi.installDir, name)
	if _, err := os.Stat(installPath); os.IsNotExist(err) {
		return fmt.Errorf("tools: plugin %q not installed", name)
	}
	return os.RemoveAll(installPath)
}

// ListInstalled 列出已安装的插件
func (pi *PluginInstaller) ListInstalled() ([]string, error) {
	entries, err := os.ReadDir(pi.installDir)
	if err != nil {
		return nil, fmt.Errorf("tools: read install dir: %w", err)
	}

	var plugins []string
	for _, entry := range entries {
		if entry.IsDir() {
			plugins = append(plugins, entry.Name())
		}
	}
	return plugins, nil
}

// InstallDir 返回安装目录
func (pi *PluginInstaller) InstallDir() string {
	return pi.installDir
}
