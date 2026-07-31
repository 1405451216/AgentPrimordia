import type { AgentTemplate, DeployConfig, DeployResult } from './types.js';
import type { TemplateRegistry } from './registry.js';

/**
 * Deployer — deploy an Agent from a template.
 * Logic aligned with Go's Deployer in internal/agent/marketplace/template.go.
 */
export class Deployer {
  private registry: TemplateRegistry;

  constructor(registry: TemplateRegistry) {
    this.registry = registry;
  }

  /** Deploy an Agent from a template, applying optional overrides. */
  deploy(cfg: DeployConfig): DeployResult {
    const tmpl = this.registry.get(cfg.template_id);
    if (!tmpl) {
      return {
        success: false,
        template_id: cfg.template_id,
        message: 'template not found',
      };
    }

    // Increment download count
    this.registry.incrementDownloads(cfg.template_id);

    // Build agent config from template
    const agentConfig: Record<string, unknown> = {
      template_id: tmpl.id,
      template_name: tmpl.name,
      system_prompt: tmpl.system_prompt,
      provider: tmpl.default_provider,
      model: tmpl.default_model,
      max_turns: tmpl.max_turns,
      tools: tmpl.tools,
      memory_strategy: tmpl.memory_strategy,
      temperature: tmpl.temperature,
    };

    // Apply overrides
    if (cfg.provider_override) {
      agentConfig.provider = cfg.provider_override;
    }
    if (cfg.model_override) {
      agentConfig.model = cfg.model_override;
    }
    if (cfg.max_turns_override && cfg.max_turns_override > 0) {
      agentConfig.max_turns = cfg.max_turns_override;
    }
    if (cfg.config_override !== undefined) {
      agentConfig.extra_config = cfg.config_override;
    }

    return {
      success: true,
      template_id: cfg.template_id,
      message: `Agent deployed from template "${tmpl.name}"`,
      agent_config: agentConfig,
    };
  }
}
