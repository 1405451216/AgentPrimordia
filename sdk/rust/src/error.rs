use thiserror::Error;

#[derive(Error, Debug)]
pub enum AgentPrimordiaError {
    #[error("API error: {message} (status: {status:?}")]
    APIError { message: String, status: Option<u16> },
    #[error("Authentication error: {0}")]
    Authentication(String),
    #[error("Rate limit exceeded: {0}")]
    RateLimit(String),
    #[error("Request timed out")]
    Timeout,
    #[error("Request failed: {0}")]
    Request(String),
    #[error("Serialization error: {0}")]
    Serde(#[from] serde_json::Error),
    #[error("IO error: {0}")]
    IO(#[from] std::io::Error),
}

impl From<reqwest::Error> for AgentPrimordiaError {
    fn from(e: reqwest::Error) -> Self {
        if e.is_timeout() { return Self::Timeout; }
        if let Some(s) = e.status() {
            match s.as_u16() {
                401 => Self::Authentication(e.to_string()),
                429 => Self::RateLimit(e.to_string()),
                code => Self::APIError { message: e.to_string(), status: Some(code) }
            }
        } else { Self::Request(e.to_string()) }
    }
}
