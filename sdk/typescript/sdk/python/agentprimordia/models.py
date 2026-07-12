"""AgentPrimordia data models."""
from dataclasses import dataclass, field
from typing import Optional, Dict, List, Any


class AgentPrimordiaError(Exception):
    """Base error for AgentPrimordia SDK."""
    def __init__(self, code: str, message: str):
        self.code = code
        self.message = message
        super().__init__(f"[{code}] {message}")


@dataclass
class Tool:
    name: str
    description: str
    parameters: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Response:
    content: str
    tokens: int = 0
    metadata: Dict[str, Any] = field(default_factory=dict)


@dataclass
class Session:
    session_id: str
    responses: List[Response] = field(default_factory=list)

    @property
    def last_response(self) -> Optional[Response]:
        return self.responses[-1] if self.responses else None


@dataclass
class Agent:
    agent_id: str
    name: str
    model: str
    session: Session