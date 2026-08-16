from dataclasses import dataclass, field
from typing import Any
from datetime import datetime, timezone

@dataclass
class Message:
    role: str  # "user" | "assistant" | "system" | "tool"
    content: str
    tool_calls: list[dict] | None = None
    metadata: dict[str, Any] = field(default_factory=dict)

@dataclass
class Tool:
    name: str
    description: str
    parameters: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {"name": self.name, "description": self.description, "parameters": self.parameters}

@dataclass
class AgentConfig:
    model: str = "gpt-4o"
    system_prompt: str = ""
    tools: list[Tool] = field(default_factory=list)
    max_turns: int = 10
    temperature: float = 0.7
    metadata: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict:
        return {
            "model": self.model,
            "system_prompt": self.system_prompt,
            "tools": [t.to_dict() for t in self.tools],
            "max_turns": self.max_turns,
            "temperature": self.temperature,
            "metadata": self.metadata,
        }

@dataclass
class Agent:
    id: str
    name: str
    config: AgentConfig
    status: str = "idle"
    created_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))  # timezone-aware UTC（utcnow 已弃用）

@dataclass
class Session:
    id: str
    agent_id: str
    messages: list[Message] = field(default_factory=list)

    @property
    def last_response(self) -> str:
        for msg in reversed(self.messages):
            if msg.role == "assistant":
                return msg.content
        return ""

    def add_message(self, role: str, content: str, **kw):
        self.messages.append(Message(role=role, content=content, **kw))
