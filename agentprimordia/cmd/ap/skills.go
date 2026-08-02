package main

import (
	"fmt"
)

const skillsUsage = `Usage: ap skill <subcommand> [arguments]

Subcommands:
  list             列出所有已习得技能
  add <file>       从 JSON/YAML 文件添加技能
  remove <id>      移除技能
  verify <id>      验证技能（运行测试用例）

Examples:
  ap skill list
  ap skill add ./my-skill.json
  ap skill remove skill-abc123
  ap skill verify skill-abc123
`

func runSkill(args []string) error {
	if len(args) == 0 {
		fmt.Print(skillsUsage)
		return nil
	}

	sub := args[0]
	subArgs := args[1:]

	switch sub {
	case "list":
		return runSkillList(subArgs)
	case "add":
		return runSkillAdd(subArgs)
	case "remove":
		return runSkillRemove(subArgs)
	case "verify":
		return runSkillVerify(subArgs)
	case "--help", "-h", "help":
		fmt.Print(skillsUsage)
		return nil
	default:
		return fmt.Errorf("unknown skill subcommand %q, run \"ap skill --help\"", sub)
	}
}

func runSkillList(args []string) error {
	_ = args
	fmt.Println("📚 技能库:")
	fmt.Println("   (暂无技能 — 通过 SDK 习得或 'ap skill add' 导入)")
	return nil
}

func runSkillAdd(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap skill add <file>")
	}
	fmt.Printf("📥 导入技能: %s\n", args[0])
	fmt.Println("   提示: 技能导入需要运行时实例，请通过 SDK 编程式使用")
	return nil
}

func runSkillRemove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap skill remove <skill-id>")
	}
	fmt.Printf("🗑️  移除技能: %s\n", args[0])
	return nil
}

func runSkillVerify(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: ap skill verify <skill-id>")
	}
	fmt.Printf("🔍 验证技能: %s\n", args[0])
	fmt.Println("   提示: 验证需要配置 SkillExecutor，请通过 SDK 编程式使用")
	return nil
}
