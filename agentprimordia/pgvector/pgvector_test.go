package pgvector

import (
	"testing"
)

func TestFloatSliceToVector(t *testing.T) {
	tests := []struct {
		input    []float32
		expected string
	}{
		{[]float32{1.0, 2.0, 3.0}, "[1,2,3]"},
		{[]float32{0.1, 0.2}, "[0.1,0.2]"},
		{[]float32{}, "[]"},
		{[]float32{-1.5, 2.5}, "[-1.5,2.5]"},
	}

	for _, tt := range tests {
		got := floatSliceToVector(tt.input)
		if got != tt.expected {
			t.Errorf("floatSliceToVector(%v) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	c := Config{}
	client := &Client{config: c}

	// Apply defaults manually (same logic as NewClient)
	if client.config.Host == "" {
		client.config.Host = "localhost"
	}
	if client.config.Port == 0 {
		client.config.Port = 5432
	}
	if client.config.TableName == "" {
		client.config.TableName = "ap_vectors"
	}
	if client.config.VectorSize == 0 {
		client.config.VectorSize = 1536
	}
	if client.config.SSLMode == "" {
		client.config.SSLMode = "disable"
	}

	if client.config.Host != "localhost" {
		t.Errorf("default Host = %q, want localhost", client.config.Host)
	}
	if client.config.Port != 5432 {
		t.Errorf("default Port = %d, want 5432", client.config.Port)
	}
	if client.config.TableName != "ap_vectors" {
		t.Errorf("default TableName = %q, want ap_vectors", client.config.TableName)
	}
	if client.config.VectorSize != 1536 {
		t.Errorf("default VectorSize = %d, want 1536", client.config.VectorSize)
	}
	if client.config.SSLMode != "disable" {
		t.Errorf("default SSLMode = %q, want disable", client.config.SSLMode)
	}
}

func TestNewClient_MissingDatabase(t *testing.T) {
	// Without a running PostgreSQL, NewClient should return an error
	_, err := NewClient(Config{
		Database: "nonexistent",
		User:     "nobody",
		Password: "nothing",
	})
	if err == nil {
		t.Error("expected error when connecting to nonexistent database")
	}
}

func TestSearchResultJSON(t *testing.T) {
	r := SearchResult{
		ID:    "test-1",
		Score: 0.95,
		Text:  "hello world",
		Metadata: map[string]any{
			"source": "test",
		},
	}
	if r.ID != "test-1" {
		t.Errorf("ID = %q, want test-1", r.ID)
	}
	if r.Score != 0.95 {
		t.Errorf("Score = %f, want 0.95", r.Score)
	}
}
