// Input validation utilities for the AgentPrimordia TypeScript SDK.
// Provides reusable validation functions for common input patterns.

export class ValidationError extends Error {
  constructor(message: string, public readonly field?: string) {
    super(message);
    this.name = 'ValidationError';
  }
}

/**
 * Validate that a string is non-empty after trimming.
 * @throws ValidationError if the value is empty
 */
export function requireNonEmpty(value: string, fieldName: string): void {
  if (!value || typeof value !== 'string' || value.trim() === '') {
    throw new ValidationError(`${fieldName} is required and must be a non-empty string`, fieldName);
  }
}

/**
 * Validate that a value is a positive integer.
 * @throws ValidationError if the value is not a positive integer
 */
export function requirePositiveInt(value: number, fieldName: string, max?: number): void {
  if (!Number.isInteger(value) || value <= 0) {
    throw new ValidationError(`${fieldName} must be a positive integer, got ${value}`, fieldName);
  }
  if (max !== undefined && value > max) {
    throw new ValidationError(`${fieldName} must be at most ${max}, got ${value}`, fieldName);
  }
}

/**
 * Validate that a value is a non-negative number.
 * @throws ValidationError if the value is negative or not a number
 */
export function requireNonNegative(value: number, fieldName: string): void {
  if (typeof value !== 'number' || !Number.isFinite(value) || value < 0) {
    throw new ValidationError(`${fieldName} must be a non-negative number, got ${value}`, fieldName);
  }
}

/**
 * Validate a URL string format.
 * @throws ValidationError if the URL is invalid
 */
export function requireValidUrl(url: string, fieldName: string): void {
  requireNonEmpty(url, fieldName);
  try {
    new URL(url);
  } catch {
    throw new ValidationError(`${fieldName} must be a valid URL, got "${url}"`, fieldName);
  }
}

/**
 * Validate an API key format (non-empty, reasonable length).
 * @throws ValidationError if the API key is invalid
 */
export function requireApiKey(apiKey: string | undefined, providerName: string): void {
  if (!apiKey || typeof apiKey !== 'string' || apiKey.trim() === '') {
    throw new ValidationError(`${providerName} API key is required`, 'apiKey');
  }
  if (apiKey.length < 10) {
    throw new ValidationError(`${providerName} API key appears too short (${apiKey.length} chars), expected at least 10`, 'apiKey');
  }
}

/**
 * Validate that a value is within a numeric range.
 * @throws ValidationError if the value is outside the range
 */
export function requireInRange(value: number, min: number, max: number, fieldName: string): void {
  if (typeof value !== 'number' || !Number.isFinite(value)) {
    throw new ValidationError(`${fieldName} must be a finite number, got ${value}`, fieldName);
  }
  if (value < min || value > max) {
    throw new ValidationError(`${fieldName} must be between ${min} and ${max}, got ${value}`, fieldName);
  }
}

/**
 * Validate a temperature value (0-2 for most LLM providers).
 * @throws ValidationError if temperature is out of range
 */
export function validateTemperature(temp: number | undefined): void {
  if (temp !== undefined) {
    requireInRange(temp, 0, 2, 'temperature');
  }
}

/**
 * Validate max tokens (must be positive, reasonable upper bound).
 * @throws ValidationError if maxTokens is invalid
 */
export function validateMaxTokens(maxTokens: number | undefined): void {
  if (maxTokens !== undefined) {
    requirePositiveInt(maxTokens, 'maxTokens', 1_000_000);
  }
}

/**
 * Validate a messages array for LLM requests.
 * @throws ValidationError if messages are invalid
 */
export function validateMessages(messages: unknown[]): void {
  if (!Array.isArray(messages) || messages.length === 0) {
    throw new ValidationError('messages must be a non-empty array', 'messages');
  }
  for (let i = 0; i < messages.length; i++) {
    const msg = messages[i];
    if (!msg || typeof msg !== 'object') {
      throw new ValidationError(`messages[${i}] must be an object`, 'messages');
    }
    const m = msg as Record<string, unknown>;
    if (!m.role || typeof m.role !== 'string') {
      throw new ValidationError(`messages[${i}].role is required and must be a string`, 'messages');
    }
    if (!['system', 'user', 'assistant', 'tool'].includes(m.role)) {
      throw new ValidationError(`messages[${i}].role must be one of: system, user, assistant, tool; got "${m.role}"`, 'messages');
    }
    if (m.content !== undefined && typeof m.content !== 'string') {
      throw new ValidationError(`messages[${i}].content must be a string if present`, 'messages');
    }
  }
}

/**
 * Validate a tool name (alphanumeric + underscores, reasonable length).
 * @throws ValidationError if the tool name is invalid
 */
export function validateToolName(name: string): void {
  requireNonEmpty(name, 'tool name');
  if (!/^[a-zA-Z_][a-zA-Z0-9_]*$/.test(name)) {
    throw new ValidationError(`tool name "${name}" must start with a letter or underscore and contain only alphanumeric characters and underscores`, 'name');
  }
  if (name.length > 128) {
    throw new ValidationError(`tool name must be at most 128 characters, got ${name.length}`, 'name');
  }
}

/**
 * Validate an agent input string.
 * @throws ValidationError if the input is invalid
 */
export function validateAgentInput(input: string): void {
  requireNonEmpty(input, 'input');
  if (input.length > 100_000) {
    throw new ValidationError(`input is too long (${input.length} chars), maximum is 100,000 characters`, 'input');
  }
}

/**
 * Validate a model name.
 * @throws ValidationError if the model name is invalid
 */
export function validateModelName(model: string | undefined, providerName: string): void {
  if (!model || model.trim() === '') {
    throw new ValidationError(`${providerName} model name is required`, 'model');
  }
  if (model.length > 200) {
    throw new ValidationError(`model name is too long (${model.length} chars)`, 'model');
  }
}
