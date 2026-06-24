package main

import (
	"fmt"
)

func runCompletion(args []string) error {
	if len(args) == 0 {
		fmt.Print(`ap completion — generate shell completion scripts

Usage:
  ap completion <shell>

Shells:
  bash        Bash completion
  zsh         Zsh completion
  fish        Fish completion

Examples:
  ap completion bash > /etc/bash_completion.d/ap
  ap completion zsh > ~/.zsh/completions/_ap
  ap completion fish > ~/.config/fish/completions/ap.fish
`)
		return nil
	}

	shell := args[0]
	switch shell {
	case "bash":
		fmt.Print(bashCompletion)
	case "zsh":
		fmt.Print(zshCompletion)
	case "fish":
		fmt.Print(fishCompletion)
	default:
		return fmt.Errorf("unsupported shell %q, supported: bash, zsh, fish", shell)
	}
	return nil
}

const bashCompletion = `# Bash completion for ap
_ap() {
    local cur prev commands
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"
    commands="init run debug test mcp plugin doctor completion version"

    if [[ ${COMP_CWORD} -eq 1 ]]; then
        COMPREPLY=($(compgen -W "${commands}" -- ${cur}))
        return 0
    fi

    case "${prev}" in
        init)
            COMPREPLY=($(compgen -W "--template --dry-run --help -t -h" -- ${cur}))
            return 0
            ;;
        run)
            COMPREPLY=($(compgen -W "--watch --prompt --help -w -p -h" -- ${cur}))
            return 0
            ;;
        debug)
            COMPREPLY=($(compgen -W "--port --help -p -h" -- ${cur}))
            return 0
            ;;
        test)
            COMPREPLY=($(compgen -W "--verbose --help -v -h" -- ${cur}))
            return 0
            ;;
        mcp)
            COMPREPLY=($(compgen -W "list add remove start stop test tools --help" -- ${cur}))
            return 0
            ;;
        plugin)
            COMPREPLY=($(compgen -W "install list create remove --help" -- ${cur}))
            return 0
            ;;
        completion)
            COMPREPLY=($(compgen -W "bash zsh fish" -- ${cur}))
            return 0
            ;;
        --template|-t)
            COMPREPLY=($(compgen -W "quickstart basic with-tools multi-agent agent-with-cache agent-with-rag agent-with-metrics" -- ${cur}))
            return 0
            ;;
        --port|-p)
            COMPREPLY=($(compgen -W "6060 8080 3000" -- ${cur}))
            return 0
            ;;
    esac
}
complete -F _ap ap
`

const zshCompletion = `#compdef ap

_ap() {
    local -a commands
    commands=(
        'init:create a new agent project'
        'run:build and run the current project'
        'debug:start debug server'
        'test:run eval test suite'
        'mcp:manage MCP servers'
        'plugin:manage plugins'
        'doctor:health check'
        'completion:generate shell completion scripts'
        'version:show version'
    )

    _arguments -C \
        '1: :->cmd' \
        '*: :->args'

    case $state in
        cmd)
            _describe 'command' commands
            ;;
        args)
            case $words[1] in
                init)
                    _arguments \
                        '(-t --template)'{-t,--template}'[template name]:template:(quickstart basic with-tools multi-agent agent-with-cache agent-with-rag agent-with-metrics)' \
                        '--dry-run[preview files without creating]' \
                        '(-h --help)'{-h,--help}'[show help]'
                    ;;
                run)
                    _arguments \
                        '(-w --watch)'{-w,--watch}'[auto-rebuild on file changes]' \
                        '(-p --prompt)'{-p,--prompt}'[send message to agent]:message:' \
                        '(-h --help)'{-h,--help}'[show help]'
                    ;;
                debug)
                    _arguments \
                        '(-p --port)'{-p,--port}'[debug server port]:port:(6060 8080 3000)' \
                        '(-h --help)'{-h,--help}'[show help]'
                    ;;
                test)
                    _arguments \
                        '(-v --verbose)'{-v,--verbose}'[show detailed output]' \
                        '(-h --help)'{-h,--help}'[show help]'
                    ;;
                mcp)
                    _arguments \
                        '1:subcommand:(list add remove start stop test tools)' \
                        '*::arg:->mcp_args'
                    ;;
                plugin)
                    _arguments \
                        '1:subcommand:(install list create remove)' \
                        '*::arg:->plugin_args'
                    ;;
                completion)
                    _arguments '1:shell:(bash zsh fish)'
                    ;;
            esac
            ;;
    esac
}

_ap "$@"
`

const fishCompletion = `# Fish completion for ap

# Subcommands
complete -c ap -n '__fish_use_subcommand' -a init -d 'create a new agent project'
complete -c ap -n '__fish_use_subcommand' -a run -d 'build and run the current project'
complete -c ap -n '__fish_use_subcommand' -a debug -d 'start debug server'
complete -c ap -n '__fish_use_subcommand' -a test -d 'run eval test suite'
complete -c ap -n '__fish_use_subcommand' -a mcp -d 'manage MCP servers'
complete -c ap -n '__fish_use_subcommand' -a plugin -d 'manage plugins'
complete -c ap -n '__fish_use_subcommand' -a doctor -d 'health check'
complete -c ap -n '__fish_use_subcommand' -a completion -d 'generate shell completions'
complete -c ap -n '__fish_use_subcommand' -a version -d 'show version'

# init options
complete -c ap -n '__fish_seen_subcommand_from init' -s t -l template -d 'template name' -a 'quickstart basic with-tools multi-agent agent-with-cache agent-with-rag agent-with-metrics'
complete -c ap -n '__fish_seen_subcommand_from init' -l dry-run -d 'preview files without creating'
complete -c ap -n '__fish_seen_subcommand_from init' -s h -l help -d 'show help'

# run options
complete -c ap -n '__fish_seen_subcommand_from run' -s w -l watch -d 'auto-rebuild on file changes'
complete -c ap -n '__fish_seen_subcommand_from run' -s p -l prompt -d 'send message to agent'
complete -c ap -n '__fish_seen_subcommand_from run' -s h -l help -d 'show help'

# debug options
complete -c ap -n '__fish_seen_subcommand_from debug' -s p -l port -d 'debug server port'
complete -c ap -n '__fish_seen_subcommand_from debug' -s h -l help -d 'show help'

# test options
complete -c ap -n '__fish_seen_subcommand_from test' -s v -l verbose -d 'show detailed output'
complete -c ap -n '__fish_seen_subcommand_from test' -s h -l help -d 'show help'

# mcp subcommands
complete -c ap -n '__fish_seen_subcommand_from mcp' -a list -d 'list registered MCP servers'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a add -d 'register a new MCP server'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a remove -d 'remove MCP server'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a start -d 'start MCP server'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a stop -d 'stop MCP server'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a test -d 'test MCP server connectivity'
complete -c ap -n '__fish_seen_subcommand_from mcp' -a tools -d 'list tools provided by MCP server'

# plugin subcommands
complete -c ap -n '__fish_seen_subcommand_from plugin' -a install -d 'install plugin from Go module'
complete -c ap -n '__fish_seen_subcommand_from plugin' -a list -d 'list installed plugins'
complete -c ap -n '__fish_seen_subcommand_from plugin' -a create -d 'create plugin project scaffold'
complete -c ap -n '__fish_seen_subcommand_from plugin' -a remove -d 'remove plugin'

# completion shells
complete -c ap -n '__fish_seen_subcommand_from completion' -a 'bash zsh fish'
`
