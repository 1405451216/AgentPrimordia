package llm

import (
	"math"
	"testing"
)

func TestModelPricing_EstimateCost(t *testing.T) {
	table := DefaultPricingTable()

	gpt4o, ok := table["gpt-4o"]
	if !ok {
		t.Fatal("DefaultPricingTable should contain gpt-4o")
	}
	if gpt4o.PromptPricePer1M <= 0 {
		t.Error("gpt-4o PromptPricePer1M should be positive")
	}
	if gpt4o.CompletionPricePer1M <= 0 {
		t.Error("gpt-4o CompletionPricePer1M should be positive")
	}
}

func TestEstimateCost_KnownModel(t *testing.T) {
	table := map[string]ModelPricing{
		"test-model": {
			Model:                "test-model",
			Provider:             "test",
			PromptPricePer1M:     5.0,
			CompletionPricePer1M: 15.0,
		},
	}

	usage := Usage{
		PromptTokens:     1000,
		CompletionTokens: 500,
		TotalTokens:      1500,
	}

	cost := EstimateCost("test-model", usage, table)

	expectedPrompt := 5.0 * 1000 / 1_000_000
	expectedCompletion := 15.0 * 500 / 1_000_000
	expected := expectedPrompt + expectedCompletion

	if math.Abs(cost-expected) > 1e-9 {
		t.Errorf("EstimateCost = %v, want %v", cost, expected)
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	table := DefaultPricingTable()
	usage := Usage{PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150}

	cost := EstimateCost("nonexistent-model", usage, table)
	if cost != 0 {
		t.Errorf("EstimateCost for unknown model should be 0, got %v", cost)
	}
}

func TestEstimateCost_ZeroUsage(t *testing.T) {
	table := map[string]ModelPricing{
		"test-model": {
			Model:                "test-model",
			PromptPricePer1M:     5.0,
			CompletionPricePer1M: 15.0,
		},
	}

	usage := Usage{}
	cost := EstimateCost("test-model", usage, table)
	if cost != 0 {
		t.Errorf("EstimateCost with zero usage should be 0, got %v", cost)
	}
}

func TestDefaultPricingTable_ContainsCommonModels(t *testing.T) {
	table := DefaultPricingTable()

	expected := []string{"gpt-4o", "gpt-4o-mini", "gpt-4-turbo", "claude-3-5-sonnet-20241022", "claude-3-haiku-20240307", "gemini-2.0-flash"}
	for _, model := range expected {
		if _, ok := table[model]; !ok {
			t.Errorf("DefaultPricingTable missing model: %s", model)
		}
	}
}

func TestModelPricing_Fields(t *testing.T) {
	p := ModelPricing{
		Model:                "test",
		Provider:             "openai",
		PromptPricePer1M:     2.5,
		CompletionPricePer1M: 10.0,
	}
	if p.Model != "test" {
		t.Error("Model field mismatch")
	}
	if p.Provider != "openai" {
		t.Error("Provider field mismatch")
	}
}
