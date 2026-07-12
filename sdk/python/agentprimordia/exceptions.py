class AgentPrimordiaError(Exception):
    """Base exception for all SDK errors."""
    def __init__(self, message: str, status_code: int | None = None, response=None):
        super().__init__(message)
        self.status_code = status_code
        self.response = response

class APIError(AgentPrimordiaError):
    """HTTP API returned non-2xx status."""
    pass

class AuthenticationError(AgentPrimordiaError):
    """Invalid or missing API key."""
    def __init__(self, message="Authentication failed. Check your API key.", **kw):
        super().__init__(message, status_code=401, **kw)

class RateLimitError(AgentPrimordiaError):
    """Too many requests."""
    def __init__(self, message="Rate limit exceeded. Try again later.", **kw):
        super().__init__(message, status_code=429, **kw)

class TimeoutError(AgentPrimordiaError):
    """Request timed out."""
    def __init__(self, message="Request timed out.", **kw):
        super().__init__(message, **kw)

class ValidationError(AgentPrimordiaError):
    """Invalid request parameters."""
    def __init__(self, message="Validation failed.", **kw):
        super().__init__(message, status_code=422, **kw)
