package plugins

import (
	ap "agentprimordia/pkg"

	emailplugin "agentprimordia/ecosystem/plugins/email"
	gitplugin "agentprimordia/ecosystem/plugins/git"
	httpplugin "agentprimordia/ecosystem/plugins/http"
	jsonplugin "agentprimordia/ecosystem/plugins/json"
	kvplugin "agentprimordia/ecosystem/plugins/kv"
	sqlplugin "agentprimordia/ecosystem/plugins/sql"
)

// LoadAll 加载所有官方插件到 Registry
// configs 为每个插件提供可选配置，key 为插件名称，value 为配置字典
func LoadAll(registry *ap.ToolRegistry, configs map[string]map[string]any) error {
	loader := ap.NewPluginLoader(registry)

	// HTTP 插件（无需配置）
	if err := loader.Load(httpplugin.New()); err != nil {
		return err
	}

	// JSON 插件（无需配置）
	if err := loader.Load(jsonplugin.New()); err != nil {
		return err
	}

	// Git 插件（配置通过构造函数传入）
	gitCfg := configs["git"]
	if gitCfg == nil {
		gitCfg = map[string]any{}
	}
	if err := loader.Load(gitplugin.New(gitCfg)); err != nil {
		return err
	}

	// SQL 插件（配置通过 Init 传入）
	sqlCfg := configs["sql"]
	if err := loader.LoadWithConfig(sqlplugin.New(), sqlCfg); err != nil {
		return err
	}

	// Email 插件（配置通过 Init 传入）
	emailCfg := configs["email"]
	if err := loader.LoadWithConfig(emailplugin.New(), emailCfg); err != nil {
		return err
	}

	// KV 插件（配置通过 Init 传入）
	kvCfg := configs["kv"]
	if err := loader.LoadWithConfig(kvplugin.New(), kvCfg); err != nil {
		return err
	}

	return nil
}
