package com.agentprimordia;

/** Represents an AI Agent instance. */
public class Agent {
    private final AgentPrimordia client;
    private final String name;
    private final String model;

    public Agent(AgentPrimordia client, String name, String model) {
        this.client = client;
        this.name = name;
        this.model = model;
    }

    public String getName() { return name; }
    public String getModel() { return model; }
    public AgentPrimordia getClient() { return client; }

    /** Placeholder for chat: actual implementation will use HttpClient. */
    public Session chat(String message) {
        return new Session();
    }
}
