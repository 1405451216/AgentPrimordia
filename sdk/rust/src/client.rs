use crate::error::AgentPrimordiaError;
use crate::models::*;
use futures::StreamExt;

pub struct AgentPrimordiaClient {
    base_url: String,
    api_key: String,
    client: reqwest::Client,
}

impl AgentPrimordiaClient {
    pub fn new(base_url: &str, api_key: &str) -> Self {
        Self {
            base_url: base_url.trim_end_matches("/").into(),
            api_key: api_key.into(),
            client: reqwest::Client::new(),
        }
    }

    pub async fn create_agent(&self, name: &str, config: &AgentConfig) -> Result<Agent, AgentPrimordiaError> {
        let url = format!("{}/api/playground/agent", self.base_url);
        let resp = self.client.post(&url).json(&serde_json::json!({ "name": name, "config": config }))
            .header("Authorization", format!("Bearer {}", self.api_key))
            .send().await?;
        if !resp.status().is_success() { return Err(resp.into()); }
        Ok(resp.json::<Agent>().await?)
    }

    pub async fn list_agents(&self) -> Result<Vec<Agent>, AgentPrimordiaError> {
        let resp = self.client.get(&format!("{}/api/playground/agents", self.base_url))
            .header("Authorization", format!("Bearer {}", self.api_key)).send().await?;
        let data: serde_json::Value = resp.json().await?;
        Ok(serde_json::from_value(data["agents"].clone()).unwrap_or_default())
    }

    pub async fn chat(&self, agent_id: &str, message: &str) -> Result<ChatResponse, AgentPrimordiaError> {
        let url = format!("{}/api/playground/agent/{}/chat", self.base_url, agent_id);
        let resp = self.client.post(&url)
            .header("Authorization", format!("Bearer {}", self.api_key))
            .json(&serde_json::json!({ "message": message }))
            .send().await?;
        Ok(resp.json().await?)
    }

    pub async fn stream(&self, agent_id: &str, message: &str) -> Result<impl futures::Stream<Item = Result<String, reqwest::Error>>, AgentPrimordiaError> {
        let url = format!("{}/api/playground/agent/{}/stream", self.base_url, agent_id);
        let resp = self.client.post(&url).json(&serde_json::json!({ "message": message })).send().await?;
        let stream = resp.bytes_stream().map(|r| r.map(|b| String::from_utf8_lossy(&b).to_string()));
        Ok(stream)
    }
}
