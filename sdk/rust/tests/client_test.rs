// client_test.rs — AgentPrimordiaClient 最小测试套件（评估报告 §8.2：Rust SDK 此前零测试）
//
// 使用 std TcpListener 本地 mock HTTP 服务，不引入额外依赖：
//   - create_agent / chat / list_agents 正常路径
//   - 401 响应 → 返回 Err（AgentPrimordiaError）
use std::io::{Read, Write};
use std::net::TcpListener;

use agentprimordia::client::AgentPrimordiaClient;
use agentprimordia::models::AgentConfig;

/// 启动一次性 mock 服务器：接受一个连接，读取请求头，按请求路径回放固定 JSON。
/// 返回服务器 base_url。
fn spawn_mock_server() -> String {
    let listener = TcpListener::bind("127.0.0.1:0").expect("bind mock server");
    let addr = listener.local_addr().expect("local addr");
    std::thread::spawn(move || {
        if let Ok((mut stream, _)) = listener.accept() {
            let mut buf = [0u8; 8192];
            let n = stream.read(&mut buf).unwrap_or(0);
            let req = String::from_utf8_lossy(&buf[..n]).into_owned();
            let (status, body) = route(&req);
            write_response(&mut stream, status, &body);
        }
    });
    format!("http://{}", addr)
}

fn route(req: &str) -> (u16, String) {
    let path = req.lines().next().unwrap_or("").to_string();
    if path.starts_with("POST /api/playground/agent ") && !path.contains("/chat") {
        (200, r#"{"id":"a1","name":"mock","status":"idle"}"#.into())
    } else if path.contains("/chat") && !path.contains("unauth") {
        (200, r#"{"response":"hi there","usage":null}"#.into())
    } else if path.contains("unauth") {
        (401, r#"{"error":"unauthorized"}"#.into())
    } else if path.starts_with("GET /api/playground/agents ") {
        (200, r#"{"agents":[{"id":"a1","name":"mock","status":"idle"}]}"#.into())
    } else {
        (404, r#"{"error":"not found"}"#.into())
    }
}

fn write_response(stream: &mut std::net::TcpStream, status: u16, body: &str) {
    let reason = match status {
        200 => "OK",
        401 => "Unauthorized",
        _ => "Not Found",
    };
    let resp = format!(
        "HTTP/1.1 {status} {reason}\r\nContent-Type: application/json\r\nContent-Length: {}\r\nConnection: close\r\n\r\n{body}",
        body.len()
    );
    let _ = stream.write_all(resp.as_bytes());
}

#[tokio::test]
async fn create_agent_parses_response() {
    let base = spawn_mock_server();
    let client = AgentPrimordiaClient::new(&base, "test-key");
    let agent = client
        .create_agent("mock", &AgentConfig::default())
        .await
        .expect("create_agent 应成功");
    assert_eq!(agent.id, "a1");
    assert_eq!(agent.name, "mock");
}

#[tokio::test]
async fn chat_returns_response() {
    let base = spawn_mock_server();
    let client = AgentPrimordiaClient::new(&base, "test-key");
    let resp = client.chat("a1", "hello").await.expect("chat 应成功");
    assert_eq!(resp.response, "hi there");
}

#[tokio::test]
async fn list_agents_parses_list() {
    let base = spawn_mock_server();
    let client = AgentPrimordiaClient::new(&base, "test-key");
    let agents = client.list_agents().await.expect("list_agents 应成功");
    assert_eq!(agents.len(), 1);
    assert_eq!(agents[0].id, "a1");
}

#[tokio::test]
async fn unauthorized_returns_error() {
    let base = spawn_mock_server();
    let client = AgentPrimordiaClient::new(&base, "bad-key");
    let err = client.chat("unauth", "x").await;
    assert!(err.is_err(), "401 应返回 Err");
    let msg = err.unwrap_err().to_string();
    assert!(msg.contains("401") || msg.contains("Unauthorized"), "错误应包含状态信息, got: {msg}");
}
