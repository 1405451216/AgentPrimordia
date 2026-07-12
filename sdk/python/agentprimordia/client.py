import json
import urllib.request
import urllib.error
import socket
from typing import Any

from .models import Agent, AgentConfig, Session, Tool, Message
from .exceptions import APIError, AuthenticationError, RateLimitError, TimeoutError

class AgentPrimordia:
    """Main SDK client for AgentPrimordia REST API."""

    def __init__(self, api_key: str = "", base_url: str = "http://localhost:3000"):
        self.api_key = api_key
        self.base_url = base_url.rstrip("/")
        self._headers = {"Content-Type": "application/json"}
        if api_key:
            self._headers["Authorization"] = f"Bearer {api_key}"

    def _request(self, method: str, path: str, body: Any = None) -> dict:
        url = f"{self.base_url}{path}"
        data = json.dumps(body).encode() if body else None
        req = urllib.request.Request(url, data=data, method=method)
        for k, v in self._headers.items():
            req.add_header(k, v)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                return json.loads(resp.read())
        except urllib.error.HTTPError as e:
            msg = e.read().decode() if e.fp else ""
            if e.code == 401: raise AuthenticationError(msg)
            if e.code == 429: raise RateLimitError(msg)
            raise APIError(msg, status_code=e.code)
        except (socket.timeout, TimeoutError):
            raise TimeoutError()

    def create_agent(self, name: str, **config_kw) -> Agent:
        """������ Agent��"""
        resp = self._request("POST", "/api/playground/agent", {"name": name, **config_kw})
        cfg = AgentConfig(**config_kw)
        return Agent(id=resp["id"], name=name, config=cfg)

    def list_agents(self) -> list[Agent]:
        """�г����� Agent��"""
        resp = self._request("GET", "/api/playground/agents")
        return [Agent(id=a["id"], name=a.get("name", ""), config=AgentConfig()) for a in resp.get("agents", [])]

    def send_message(self, agent_id: str, message: str) -> str:
        """�� Agent ������Ϣ����ȡ�ظ���"""
        resp = self._request("POST", f"/api/playground/agent/{agent_id}/chat", {"message": message})
        return resp.get("response", "")

    def stream_chat(self, agent_id: str, message: str):
        """��ʽ���죨�� chunk yield����"""
        url = f"{self.base_url}/api/playground/agent/{agent_id}/stream"
        req = urllib.request.Request(url, method="POST")
        for k, v in self._headers.items():
            req.add_header(k, v)
        req.add_header("Content-Type", "application/json")
        req.data = json.dumps({"message": message}).encode()
        with urllib.request.urlopen(req, timeout=300) as resp:
            for line in resp:
                line = line.decode().strip()
                if line.startswith("data: "):
                    yield line[6:]

    def get_stats(self, agent_id: str) -> dict:
        """��ȡ Agent ͳ����Ϣ��"""
        return self._request("GET", f"/api/playground/agent/{agent_id}/stats")

    def delete_agent(self, agent_id: str):
        """ɾ�� Agent��"""
        self._request("DELETE", f"/api/playground/agent/{agent_id}")

    # Tool ����
    def register_tool(self, name: str, description: str, parameters: dict | None = None) -> Tool:
        """ע��ȫ�ֹ��ߡ�"""
        tool = Tool(name=name, description=description, parameters=parameters or {})
        self._request("POST", "/api/tools", tool.to_dict())
        return tool

    # Memory ����
    def query_memory(self, agent_id: str, query: str, top_k: int = 5) -> list[dict]:
        """��ѯ Agent ���䡣"""
        resp = self._request("POST", f"/api/agent/{agent_id}/memory/query", {"query": query, "top_k": top_k})
        return resp.get("results", [])

    # Session
    def create_session(self, agent_id: str) -> Session:
        resp = self._request("POST", f"/api/agent/{agent_id}/session", {})
        return Session(id=resp["id"], agent_id=agent_id)

    def get_session_messages(self, session_id: str) -> list[Message]:
        resp = self._request("GET", f"/api/session/{session_id}/messages")
        return [Message(**m) for m in resp.get("messages", [])]

    # Convenience: chainable interaction
    def chat(self, agent_id: str, message: str, stream: bool = False) -> str:
        if stream:
            chunks = []
            for chunk in self.stream_chat(agent_id, message):
                chunks.append(chunk)
            return "".join(chunks)
        return self.send_message(agent_id, message)
