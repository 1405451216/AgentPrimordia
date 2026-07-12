"""AgentPrimordia HTTP client."""
import json
import urllib.request
import urllib.error
from typing import Optional, Dict, Any

from .models import Agent, AgentPrimordiaError, Response, Session, Tool


class AgentPrimordia:
    """Main client for interacting with AgentPrimordia API."""

    def __init__(self, api_key: str, base_url: str = "http://localhost:8080"):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")

    def create_agent(
        self,
        name: str,
        model: str = "gpt-4",
        system_prompt: Optional[str] = None,
        tools: Optional[list] = None,
        max_turns: int = 10,
    ) -> Agent:
        body = json.dumps({
            "name": name,
            "model": model,
            "system_prompt": system_prompt,
            "tools": tools or [],
            "max_turns": max_turns,
        }).encode("utf-8")
        resp = self._request("POST", "/api/playground/agents", body)
        return Agent(
            agent_id=resp["id"],
            name=name,
            model=model,
            session=Session(session_id=resp.get("session_id", resp["id"])),
        )

    def chat(self, agent: Agent, message: str) -> Session:
        body = json.dumps({"message": message}).encode("utf-8")
        path = "/api/playground/agents/" + agent.agent_id + "/chat"
        resp = self._request("POST", path, body)
        r = Response(content=resp.get("response", ""), tokens=resp.get("tokens", 0))
        agent.session.responses.append(r)
        return agent.session

    def list_agents(self) -> list:
        resp = self._request("GET", "/api/playground/agents")
        return [
            Agent(
                agent_id=a["id"],
                name=a.get("name", a["id"]),
                model=a.get("model", ""),
                session=Session(session_id=a["id"]),
            )
            for a in resp
        ]

    def _request(self, method: str, path: str, body: Optional[bytes] = None) -> Any:
        url = self.base_url + path
        req = urllib.request.Request(url, data=body, method=method)
        req.add_header("Content-Type", "application/json")
        req.add_header("Authorization", "Bearer " + self.api_key)
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                return json.loads(resp.read().decode("utf-8"))
        except urllib.error.HTTPError as e:
            raise AgentPrimordiaError("HTTP_" + str(e.code), e.reason) from e
        except urllib.error.URLError as e:
            raise AgentPrimordiaError("CONNECTION_ERROR", str(e.reason)) from e
