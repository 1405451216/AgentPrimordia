package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runPlugin(args []string) error {
	if len(args) == 0 {
		fmt.Print(`ap plugin — manage plugins

Usage:
  ap plugin <subcommand> [arguments]

Subcommands:
  install <module>   install plugin from Go module
  list               list installed plugins
  create <name>      create plugin project scaffold
  remove <name>      remove plugin

Examples:
  ap plugin install github.com/user/ap-plugin-slack
  ap plugin list
  ap plugin create ap-plugin-weather
`)
		return nil
	}

	subcmd := args[0]
	subargs := args[1:]

	switch subcmd {
	case "install":
		return pluginInstall(subargs)
	case "list":
		return pluginList()
	case "create":
		return pluginCreate(subargs)
	case "remove":
		return pluginRemove(subargs)
	default:
		return fmt.Errorf("unknown subcommand: %s", subcmd)
	}
}

func pluginInstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please specify Go module path\nUsage: ap plugin install github.com/user/ap-plugin-xxx")
	}

	module := args[0]
	dir, err := findProjectDir()
	if err != nil {
		return err
	}

	infof("installing plugin: %s", module)
	cmd := exec.Command("go", "get", module)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("install failed: %w", err)
	}

	config := loadAPConfig()
	if config.Plugins == nil {
		config.Plugins = []string{}
	}
	config.Plugins = append(config.Plugins, module)
	if err := saveAPConfig(config); err != nil {
		warnf("save config failed: %v", err)
	}

	successf("plugin %s installed", module)
	fmt.Println()
	fmt.Println("import the plugin in your code:")
	fmt.Printf("  import _ %q\n", module)
	fmt.Printf("  // then in init(): pluginLoader.Load(%q.NewPlugin())\n", module)
	fmt.Println()
	fmt.Println("run ap run to activate the plugin")
	return nil
}

func pluginList() error {
	config := loadAPConfig()
	if len(config.Plugins) == 0 {
		fmt.Println("no plugins installed")
		fmt.Println()
		fmt.Println("use ap plugin install <module> to install plugins")
		return nil
	}

	fmt.Printf("%-40s %s\n", "Module Path", "Status")
	fmt.Println(strings.Repeat("-", 60))
	for _, p := range config.Plugins {
		fmt.Printf("%-40s %s\n", p, "installed")
	}
	return nil
}

func pluginCreate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please specify plugin name\nUsage: ap plugin create ap-plugin-xxx")
	}

	name := args[0]
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("directory %q already exists", name)
	}

	dirs := []string{name, filepath.Join(name, "tools")}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("create directory failed: %w", err)
		}
	}

	// go.mod
	goMod := fmt.Sprintf(`module %s

go 1.23

require agentprimordia v1.0.0
`, name)
	if err := os.WriteFile(filepath.Join(name, "go.mod"), []byte(goMod), 0o644); err != nil {
		return fmt.Errorf("write go.mod failed: %w", err)
	}

	// plugin.go
	pluginCode := fmt.Sprintf(`package %s

import (
	ap "agentprimordia/pkg"
)

// Plugin implements ap.ToolPlugin interface.
type Plugin struct{}

// NewPlugin creates a plugin instance (entry point).
func NewPlugin() ap.ToolPlugin {
	return &Plugin{}
}

// Name returns the plugin name.
func (p *Plugin) Name() string {
	return %q
}

// Version returns the plugin version.
func (p *Plugin) Version() string {
	return "0.1.0"
}

// Tools returns the tools provided by this plugin.
func (p *Plugin) Tools() []ap.Tool {
	return []ap.Tool{
		// register your tools here
	}
}

// Init initializes the plugin.
func (p *Plugin) Init(config map[string]any) error {
	return nil
}

// Close cleans up resources.
func (p *Plugin) Close() error {
	return nil
}
`, strings.ReplaceAll(name, "-", "_"), name)
	if err := os.WriteFile(filepath.Join(name, "plugin.go"), []byte(pluginCode), 0o644); err != nil {
		return fmt.Errorf("write plugin.go failed: %w", err)
	}

	// README
	readme := "# " + name + "\n\nAgentPrimordia plugin.\n\n" +
		"## Install\n\n" +
		"```bash\nap plugin install " + name + "\n```\n\n" +
		"## Usage\n\n" +
		"```go\nimport _ \"" + name + "\"\n\n// plugin auto-registers to ToolRegistry\n```\n\n" +
		"## Development\n\n" +
		"```bash\ncd " + name + "\ngo mod tidy\ngo test ./...\n```\n"
	if err := os.WriteFile(filepath.Join(name, "README.md"), []byte(readme), 0o644); err != nil {
		return fmt.Errorf("write README.md failed: %w", err)
	}

	successf("plugin project %q created", name)
	fmt.Println()
	fmt.Printf("Directory structure:\n")
	fmt.Printf("  %s/\n", name)
	fmt.Printf("  ├── plugin.go    — plugin entry (implements ToolPlugin)\n")
	fmt.Printf("  ├── go.mod\n")
	fmt.Printf("  └── README.md\n\n")
	fmt.Printf("Next steps:\n")
	infof("cd %s", name)
	infof("# edit plugin.go to add your tools")
	infof("go mod tidy")
	return nil
}

func pluginRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("please specify plugin name")
	}

	module := args[0]
	config := loadAPConfig()

	found := false
	var newPlugins []string
	for _, p := range config.Plugins {
		if p == module {
			found = true
			continue
		}
		newPlugins = append(newPlugins, p)
	}

	if !found {
		return fmt.Errorf("plugin %q not installed", module)
	}

	config.Plugins = newPlugins
	if err := saveAPConfig(config); err != nil {
		return fmt.Errorf("save config failed: %w", err)
	}

	successf("plugin %q removed from config", module)

	// auto-run go mod tidy
	dir, dirErr := findProjectDir()
	if dirErr == nil {
		infof("running go mod tidy...")
		if _, tidyErr := runCommand(dir, "go", "mod", "tidy"); tidyErr != nil {
			warnf("go mod tidy failed: %v (run manually)", tidyErr)
		} else {
			successf("go mod tidy completed")
		}
	}
	return nil
}
