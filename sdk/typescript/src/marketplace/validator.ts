import type { AgentTemplate, ValidationResult } from './types.js';

const VALID_CATEGORIES = new Set(['research', 'coding', 'analysis', 'chat', 'automation']);
const VALID_MEMORY_STRATEGIES = new Set(['', 'none', 'conversation', 'semantic', 'hybrid']);

/**
 * Validate a template against the same rules as Go's AgentTemplate.Validate().
 * Returns a ValidationResult with errors and security warnings.
 */
export function validateTemplate(t: AgentTemplate): ValidationResult {
  const result: ValidationResult = { valid: true };
  const errors: string[] = [];
  const securityWarnings: string[] = [];

  if (!t.id) {
    result.valid = false;
    errors.push('id is required');
  }
  if (!t.name) {
    result.valid = false;
    errors.push('name is required');
  }
  if (!t.version) {
    result.valid = false;
    errors.push('version is required');
  }
  if (!t.author) {
    result.valid = false;
    errors.push('author is required');
  }
  if (!t.system_prompt) {
    result.valid = false;
    errors.push('system_prompt is required');
  }

  // Category validation
  if (t.category && !VALID_CATEGORIES.has(t.category)) {
    result.valid = false;
    errors.push(`invalid category: ${t.category}`);
  }

  // Memory strategy validation
  const ms = t.memory_strategy ?? '';
  if (!VALID_MEMORY_STRATEGIES.has(ms)) {
    result.valid = false;
    errors.push(`invalid memory_strategy: ${ms}`);
  }

  // Temperature validation (0-2)
  const temp = t.temperature ?? 0;
  if (temp < 0 || temp > 2) {
    result.valid = false;
    errors.push(`temperature must be 0-2, got ${temp}`);
  }

  // Rating validation (0-5)
  if (t.rating < 0 || t.rating > 5) {
    result.valid = false;
    errors.push(`rating must be 0-5, got ${t.rating}`);
  }

  // MaxTurns validation
  const maxTurns = t.max_turns ?? 0;
  if (maxTurns < 0) {
    result.valid = false;
    errors.push('max_turns must be non-negative');
  }

  // Security scanning — dangerous commands
  if (t.system_prompt.includes('rm -rf')) {
    securityWarnings.push('system_prompt contains potentially dangerous command');
  }
  // Security scanning — prompt injection
  if (t.system_prompt.toLowerCase().includes('ignore previous')) {
    securityWarnings.push('system_prompt contains prompt injection pattern');
  }

  if (errors.length > 0) result.errors = errors;
  if (securityWarnings.length > 0) result.security_warnings = securityWarnings;

  return result;
}
