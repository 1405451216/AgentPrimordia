package main

import (
	"fmt"
	"os"
)

var Version = "dev"

const (
	usage = `AgentPrimordia (ap) — Go Agent Framework CLI

Usage:
  ap <command> [arguments]

Commands:
  init         create a new agent project
  run          build and run the current project
  debug        start debug server
  test         run eval test suite
  mcp          manage MCP servers
  plugin       manage plugins
  doctor       health check
  completion   generate shell completion scripts
  version      show version

Run "ap <command> --help" for subcommand details.
`
)

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(0)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	switch cmd {
	case "init":
		runInit(args)
	case "run":
		runRun(args)
	case "debug":
		runDebug(args)
	case "test":
		runTest(args)
	case "mcp":
		runMCP(args)
	case "plugin":
		runPlugin(args)
	case "doctor":
		runDoctor(args)
	case "completion":
		runCompletion(args)
	case "version", "-v", "--version":
		fmt.Printf("AgentPrimordia CLI %s\n", Version)
	default:
		errorf("unknown command %q, run %s to see available commands", cmd, bold("ap --help"))
		os.Exit(1)
	}
}
