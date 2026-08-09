# -*- coding: utf-8 -*-
"""AgentPrimordia Python SDK 最小测试套件（评估报告 §8.2：Python SDK 此前零测试）。

使用 stdlib http.server 起本地 mock 服务，无第三方测试依赖：
  - create_agent / send_message / list_agents / get_stats 正常路径
  - 401 响应 → AuthenticationError
运行：python -m unittest discover -s tests -v（cwd = sdk/python）
"""
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from agentprimordia.client import AgentPrimordia
from agentprimordia.exceptions import AuthenticationError, APIError


class MockHandler(BaseHTTPRequestHandler):
    """按路径回放固定 JSON 的 mock 服务。"""

    def _send(self, code, obj):
        body = json.dumps(obj).encode()
        self.send_response(code)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length", 0) or 0)
        if length:
            self.rfile.read(length)
        if self.path == "/api/playground/agent":
            self._send(200, {"id": "a1", "name": "mock", "config": {}})
        elif "/chat" in self.path:
            if "unauth" in self.path:
                self._send(401, {"error": "unauthorized"})
            else:
                self._send(200, {"response": "hi there", "usage": None})
        else:
            self._send(404, {"error": "not found"})

    def do_GET(self):
        if self.path == "/api/playground/agents":
            self._send(200, {"agents": [{"id": "a1", "name": "mock"}]})
        elif self.path == "/api/playground/agent/a1/stats":
            self._send(200, {"turns": 3, "tokens": 100})
        else:
            self._send(404, {"error": "not found"})

    def log_message(self, *args):  # 静默访问日志
        pass


class TestClient(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), MockHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        host, port = cls.server.server_address
        cls.base = f"http://{host}:{port}"
        cls.client = AgentPrimordia(api_key="test-key", base_url=cls.base)

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def test_create_agent(self):
        agent = self.client.create_agent("mock")
        self.assertEqual(agent.id, "a1")
        self.assertEqual(agent.name, "mock")

    def test_send_message(self):
        resp = self.client.send_message("a1", "hello")
        self.assertEqual(resp, "hi there")

    def test_list_agents(self):
        agents = self.client.list_agents()
        self.assertEqual(len(agents), 1)
        self.assertEqual(agents[0].id, "a1")

    def test_get_stats(self):
        stats = self.client.get_stats("a1")
        self.assertEqual(stats["turns"], 3)

    def test_unauthorized_raises_auth_error(self):
        with self.assertRaises(AuthenticationError):
            self.client.send_message("unauth", "x")

    def test_not_found_raises_api_error(self):
        with self.assertRaises(APIError):
            self.client.get_stats("missing")


if __name__ == "__main__":
    unittest.main()
