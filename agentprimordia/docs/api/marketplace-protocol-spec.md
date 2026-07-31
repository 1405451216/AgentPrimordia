# Agent Marketplace Protocol Specification

## Overview

This document defines the JSON schema and validation rules for Agent templates
in the AgentPrimordia Marketplace. All third-party templates must conform to this
specification and pass the `Validate()` security scan before registration.

---

## AgentTemplate JSON Schema

### Required Fields

| Field         | Type    | JSON Key        | Description                          | Constraints                        |
|---------------|---------|-----------------|--------------------------------------|------------------------------------|
| ID            | string  | `id`            | Template unique identifier           | Non-empty                          |
| Name          | string  | `name`          | Template display name                | Non-empty                          |
| Description   | string  | `description`   | Template description                 | —                                  |
| Version       | string  | `version`       | Semantic version (e.g. `1.0.0`)      | Non-empty                          |
| Author        | string  | `author`        | Template author name or org          | Non-empty                          |
| Category      | string  | `category`      | Template category                    | Must be one of the enum values below (or empty) |
| SystemPrompt  | string  | `system_prompt` | Agent system prompt                  | Non-empty; must pass security scan |
| Rating        | float   | `rating`        | Average user rating                  | Range: 0.0 – 5.0                  |
| Downloads     | int     | `downloads`     | Total download count                 | Non-negative                       |
| CreatedAt     | string  | `created_at`    | ISO-8601 creation timestamp          | —                                  |
| UpdatedAt     | string  | `updated_at`    | ISO-8601 last-update timestamp       | —                                  |

### Optional Fields

| Field           | Type      | JSON Key           | Description                        | Constraints                        |
|-----------------|-----------|--------------------|------------------------------------|------------------------------------|
| Tags            | []string  | `tags,omitempty`   | Searchable tags                    | —                                  |
| DefaultProvider | string    | `default_provider,omitempty` | Default LLM provider     | —                                  |
| DefaultModel    | string    | `default_model,omitempty`    | Default model name       | —                                  |
| MaxTurns        | int       | `max_turns,omitempty`        | Maximum conversation turns | Must be >= 0                    |
| Tools           | []string  | `tools,omitempty`            | Bound tool identifiers | —                                  |
| MemoryStrategy  | string    | `memory_strategy,omitempty`  | Memory strategy          | Must be one of the enum values below |
| Temperature     | float     | `temperature,omitempty`      | LLM sampling temperature | Range: 0.0 – 2.0                 |
| Config          | object    | `config,omitempty`           | Extra JSON configuration | Arbitrary JSON                     |

---

## Enum Values

### Category

| Value        | Description                        |
|--------------|------------------------------------|
| `research`   | Research and information retrieval |
| `coding`     | Code generation and review         |
| `analysis`   | Data analysis and visualization    |
| `chat`       | General conversational agent       |
| `automation` | Workflow automation                |

An empty category is also accepted (treated as uncategorised).

### MemoryStrategy

| Value          | Description                                     |
|----------------|-------------------------------------------------|
| `none`         | No persistent memory                            |
| `conversation` | Sliding-window conversation history             |
| `semantic`     | Vector-based semantic recall                      |
| `hybrid`       | Combination of conversation + semantic memory   |

An empty value (field omitted) is also accepted.

---

## Security Scanning Rules

The `Validate()` method performs the following security checks on every template:

| Rule | Trigger | Warning Message |
|------|---------|-----------------|
| Dangerous command detection | `system_prompt` contains `rm -rf` | `system_prompt contains potentially dangerous command` |
| Prompt injection detection | `system_prompt` contains `ignore previous` (case-insensitive) | `system_prompt contains prompt injection pattern` |

Templates that trigger security warnings are **not rejected** but flagged for
manual review. Registry operators should inspect the warnings before approving
third-party templates.

---

## Validation Summary

The `Validate()` method returns a `ValidationResult`:

```json
{
  "valid": true,
  "errors": [],
  "security_warnings": []
}
```

- `valid` is `false` when **any** required field is missing or a constraint is violated.
- `errors` lists all validation failures.
- `security_warnings` lists security concerns (non-blocking).

---

## Example Template

```json
{
  "id": "example-agent",
  "name": "Example Agent",
  "description": "A minimal example template",
  "version": "1.0.0",
  "author": "AgentPrimordia Team",
  "category": "chat",
  "tags": ["example", "demo"],
  "system_prompt": "You are a helpful assistant.",
  "default_provider": "openai",
  "default_model": "gpt-4",
  "max_turns": 50,
  "tools": ["web_search", "calculator"],
  "memory_strategy": "conversation",
  "temperature": 0.7,
  "rating": 4.5,
  "downloads": 1000,
  "created_at": "2026-01-15T10:00:00Z",
  "updated_at": "2026-06-01T12:00:00Z"
}
```
