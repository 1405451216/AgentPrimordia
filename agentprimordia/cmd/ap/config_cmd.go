package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigHelp()
		return nil
	}

	subcmd := args[0]
	switch subcmd {
	case "validate":
		return runConfigValidate(args[1:])
	case "--help", "-h", "help":
		printConfigHelp()
		return nil
	default:
		return fmt.Errorf("unknown config subcommand %q, run %s for help", subcmd, bold("ap config --help"))
	}
}

func runConfigValidate(args []string) error {
	configPath := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--file", "-f":
			i++
			if i >= len(args) {
				return fmt.Errorf("--file requires a path argument")
			}
			configPath = args[i]
		case "--help", "-h":
			fmt.Print(`ap config validate — validate .ap.yaml configuration

Usage:
  ap config validate [--file PATH]

Options:
  --file, -f PATH    specify config file path (default: .ap.yaml in project root)

Examples:
  ap config validate
  ap config validate --file /path/to/.ap.yaml
`)
			return nil
		}
	}

	// Determine config file path
	if configPath == "" {
		dir, err := findProjectDir()
		if err != nil {
			return fmt.Errorf("could not find project directory: %w", err)
		}
		configPath = filepath.Join(dir, ".ap.yaml")
	}

	// Check file exists
	if _, err := os.Stat(configPath); err != nil {
		if os.IsNotExist(err) {
			// Try .ap.json as fallback
			jsonPath := strings.TrimSuffix(configPath, ".yaml") + ".json"
			if _, jsonErr := os.Stat(jsonPath); jsonErr == nil {
				configPath = jsonPath
			} else {
				return fmt.Errorf("config file not found: %s", configPath)
			}
		} else {
			return fmt.Errorf("cannot access config file: %w", err)
		}
	}

	// Read and parse
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg apConfig
	if strings.HasSuffix(configPath, ".json") {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse JSON config: %w", err)
		}
	} else {
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse YAML config: %w", err)
		}
	}

	// Run validation
	errs := cfg.Validate()
	if len(errs) == 0 {
		successf("Configuration is valid: %s", configPath)

		// Print summary
		if cfg.Name != "" {
			fmt.Printf("  name:     %s\n", cfg.Name)
		}
		if cfg.LLM != nil {
			fmt.Printf("  llm:      provider=%s, model=%s\n", cfg.LLM.Provider, cfg.LLM.Model)
		}
		if cfg.Agent != nil {
			fmt.Printf("  agent:    max_turns=%d\n", cfg.Agent.MaxTurns)
		}
		if cfg.Memory != nil {
			fmt.Printf("  memory:   backend=%s\n", cfg.Memory.Backend)
		}
		if len(cfg.Plugins) > 0 {
			fmt.Printf("  plugins:  %s\n", strings.Join(cfg.Plugins, ", "))
		}
		if cfg.MCP != nil && len(cfg.MCP.Servers) > 0 {
			fmt.Printf("  mcp:      %d server(s)\n", len(cfg.MCP.Servers))
		}
		return nil
	}

	// Report errors
	errorf("Configuration has %d error(s):", len(errs))
	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "  • %s\n", e)
	}
	return fmt.Errorf("configuration validation failed with %d error(s)", len(errs))
}

func printConfigHelp() {
	fmt.Print(`ap config — manage agent configuration

Usage:
  ap config <subcommand> [options]

Subcommands:
  validate       validate .ap.yaml configuration file

Run "ap config <subcommand> --help" for details.
`)
}
