/**
 * Agent Marketplace types — aligned with Go internal/agent/marketplace/template.go.
 * JSON field names use snake_case to match Go struct tags exactly.
 */

/** Agent template definition (mirrors Go AgentTemplate struct) */
export interface AgentTemplate {
  id: string;
  name: string;
  description: string;
  version: string;
  author: string;
  /** "research" | "coding" | "analysis" | "chat" | "automation" */
  category: string;
  tags?: string[];
  system_prompt: string;
  default_provider?: string;
  default_model?: string;
  max_turns?: number;
  tools?: string[];
  /** "none" | "conversation" | "semantic" | "hybrid" */
  memory_strategy?: string;
  temperature?: number;
  config?: unknown;
  rating: number;
  downloads: number;
  created_at: string;
  updated_at: string;
}

/** Valid template categories */
export type TemplateCategory = 'research' | 'coding' | 'analysis' | 'chat' | 'automation';

/** Valid memory strategies */
export type MemoryStrategy = 'none' | 'conversation' | 'semantic' | 'hybrid';

/** Template validation result (mirrors Go ValidationResult) */
export interface ValidationResult {
  valid: boolean;
  errors?: string[];
  security_warnings?: string[];
}

/** Deploy configuration (mirrors Go DeployConfig) */
export interface DeployConfig {
  template_id: string;
  provider_override?: string;
  model_override?: string;
  max_turns_override?: number;
  config_override?: unknown;
}

/** Deploy result (mirrors Go DeployResult) */
export interface DeployResult {
  success: boolean;
  template_id: string;
  message: string;
  agent_config?: Record<string, unknown>;
}
