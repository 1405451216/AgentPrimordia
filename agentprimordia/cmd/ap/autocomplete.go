// cmd/ap/autocomplete.go - shell 自动补全脚本生成器
//
// 提供 bash / fish / powershell 三种 shell 的自动补全脚本生成函数。
// 已有 zsh 补全见 completion.go（runCompletion 命令分发）。
// 本文件将各 shell 补全脚本封装为可独立调用的导出函数，
// 便于测试和外部程序（如 middleware 链路中的 help 端点）复用。
package main

import "strings"

// GenerateBashCompletion 返回 bash 自动补全脚本。
func GenerateBashCompletion() string {
	var b strings.Builder
	b.WriteString("# bash completion for ap\n")
	b.WriteString("_ap() {\n")
	b.WriteString("    local cur prev commands\n")
	b.WriteString("    COMPREPLY=()\n")
	b.WriteString("    cur=\"${COMP_WORDS[COMP_CWORD]}\"\n")
	b.WriteString("    prev=\"${COMP_WORDS[COMP_CWORD-1]}\"\n")
	b.WriteString("    commands=\"init run debug loop test config mcp plugin doctor completion version\"\n")
	b.WriteString("    if [[ ${COMP_CWORD} -eq 1 ]]; then\n")
	b.WriteString("        COMPREPLY=($(compgen -W \"${commands}\" -- ${cur}))\n")
	b.WriteString("        return 0\n")
	b.WriteString("    fi\n")
	b.WriteString("    case \"${prev}\" in\n")
	b.WriteString("        init)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"--template --dry-run --help -t -h\" -- ${cur}));;\n")
	b.WriteString("        run)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"--watch --prompt --help -w -p -h\" -- ${cur}));;\n")
	b.WriteString("        debug)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"--port --help -p -h\" -- ${cur}));;\n")
	b.WriteString("        loop)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"--trace --inspect --resume --help\" -- ${cur}));;\n")
	b.WriteString("        test)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"--verbose --help -v -h\" -- ${cur}));;\n")
	b.WriteString("        config)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"get set list --help\" -- ${cur}));;\n")
	b.WriteString("        mcp)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"list add remove start stop test tools --help\" -- ${cur}));;\n")
	b.WriteString("        plugin)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"install list create remove --help\" -- ${cur}));;\n")
	b.WriteString("        completion)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"bash fish powershell zsh\" -- ${cur}));;\n")
	b.WriteString("        --template|-t)\n")
	b.WriteString("            COMPREPLY=($(compgen -W \"quickstart basic with-tools multi-agent agent-with-cache agent-with-rag agent-with-metrics\" -- ${cur}));;\n")
	b.WriteString("    esac\n")
	b.WriteString("}\n")
	b.WriteString("complete -F _ap ap\n")
	return b.String()
}

// GenerateFishCompletion 返回 fish 自动补全脚本。
func GenerateFishCompletion() string {
	var b strings.Builder
	b.WriteString("# fish completion for ap\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a init -d 'create a new agent project'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a run -d 'build and run the current project'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a debug -d 'start debug server'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a loop -d 'ReAct loop engineering'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a test -d 'run eval test suite'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a config -d 'manage configuration'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a mcp -d 'manage MCP servers'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a plugin -d 'manage plugins'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a doctor -d 'health check'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a completion -d 'generate shell completions'\n")
	b.WriteString("complete -c ap -n '__fish_use_subcommand' -a version -d 'show version'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from init' -s t -l template -d 'template name' -a 'quickstart basic with-tools multi-agent agent-with-cache agent-with-rag agent-with-metrics'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from init' -l dry-run -d 'preview files without creating'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from init' -s h -l help -d 'show help'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from run' -s w -l watch -d 'auto-rebuild on file changes'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from run' -s p -l prompt -d 'send message to agent'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from run' -s h -l help -d 'show help'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from debug' -s p -l port -d 'debug server port'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from debug' -s h -l help -d 'show help'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a list -d 'list registered MCP servers'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a add -d 'register a new MCP server'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a remove -d 'remove MCP server'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a start -d 'start MCP server'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a stop -d 'stop MCP server'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a test -d 'test MCP server connectivity'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from mcp' -a tools -d 'list tools provided by MCP server'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from plugin' -a install -d 'install plugin from Go module'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from plugin' -a list -d 'list installed plugins'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from plugin' -a create -d 'create plugin project scaffold'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from plugin' -a remove -d 'remove plugin'\n")
	b.WriteString("complete -c ap -n '__fish_seen_subcommand_from completion' -a 'bash fish powershell zsh'\n")
	return b.String()
}

// GeneratePowerShellCompletion 返回 PowerShell 自动补全脚本。
func GeneratePowerShellCompletion() string {
	var b strings.Builder
	b.WriteString("# PowerShell completion for ap\n")
	b.WriteString("function _ap_completions {\n")
	b.WriteString("    param($wordToComplete, $commandAst, $cursorPosition)\n")
	b.WriteString("    $commands = @(\n")
	b.WriteString("        \"init\", \"run\", \"debug\", \"loop\", \"test\",\n")
	b.WriteString("        \"config\", \"mcp\", \"plugin\", \"doctor\",\n")
	b.WriteString("        \"completion\", \"version\"\n")
	b.WriteString("    )\n")
	b.WriteString("    $commands | Where-Object { $_ -like \"$wordToComplete*\" } | ForEach-Object {\n")
	b.WriteString("        [System.Management.Automation.CompletionResult]::new($_, $_, \"ParameterValue\", $_)\n")
	b.WriteString("    }\n")
	b.WriteString("}\n")
	b.WriteString("Register-ArgumentCompleter -Native -CommandName ap -ScriptBlock _ap_completions\n")
	return b.String()
}
