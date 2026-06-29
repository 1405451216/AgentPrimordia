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
  loop         ReAct loop engineering (trace/inspect/resume)
  test         run eval test suite
  config       manage configuration
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

	var err error
	switch cmd {
	case "init":
		err = runInit(args)
	case "run":
		err = runRun(args)
	case "debug":
		err = runDebug(args)
	case "loop":
		err = runLoop(args)
	case "test":
		err = runTest(args)
	case "config":
		err = runConfig(args)
	case "mcp":
		err = runMCP(args)
	case "plugin":
		err = runPlugin(args)
	case "doctor":
		err = runDoctor(args)
	case "completion":
		err = runCompletion(args)
	case "version", "-v", "--version":
		fmt.Printf("AgentPrimordia CLI %s\n", Version)
	case "--help", "-h", "help":
		fmt.Print(usage)
	default:
		errorf("unknown command %q, run %s to see available commands", cmd, bold("ap --help"))
		os.Exit(1)
	}

	if err != nil {
		errorf("%v", err)
		os.Exit(1)
	}
}
