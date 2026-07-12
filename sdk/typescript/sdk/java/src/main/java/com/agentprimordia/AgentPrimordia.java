package com.agentprimordia;

import java.util.List;
import java.util.ArrayList;

/** Main entry point for AgentPrimordia Java SDK. */
public class AgentPrimordia {
    private final String apiKey;
    private final String baseUrl;

    public AgentPrimordia(String apiKey) {
        this(apiKey, "http://localhost:8080");
    }

    public AgentPrimordia(String apiKey, String baseUrl) {
        this.apiKey = apiKey;
        this.baseUrl = baseUrl.replaceAll("/+$", "");
    }

    public Agent createAgent(String name, String model) {
        return new Agent(this, name, model);
    }

    public String getApiKey() { return apiKey; }
    public String getBaseUrl() { return baseUrl; }

    public static void main(String[] args) {
        AgentPrimordia client = new AgentPrimordia("test-key");
        Agent agent = client.createAgent("assistant", "gpt-4");
        System.out.println("Agent created: " + agent.getName());
    }
}
