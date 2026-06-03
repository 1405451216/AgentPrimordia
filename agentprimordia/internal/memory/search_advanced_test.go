package memory

import (
	"context"
	"testing"
)

func TestSQLiteStore_SearchAdvanced_Basic(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "The quick brown fox jumps over the lazy dog"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "I like programming in Go"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:      "fox",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchAdvanced results length = %d, want %d", len(results), 1)
	}
	if results[0].Episode.Content != "The quick brown fox jumps over the lazy dog" {
		t.Errorf("unexpected content: %s", results[0].Episode.Content)
	}
	if results[0].KeywordScore <= 0 {
		t.Errorf("KeywordScore should be > 0, got %f", results[0].KeywordScore)
	}
}

func TestSQLiteStore_SearchAdvanced_WithTags(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Message about Go programming")
	ep1.Topics = "go,programming"
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Message about Python data science")
	ep2.Topics = "python,data"
	_ = store.Add(ctx, ep2)

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:      "programming",
		Tags:       []string{"go"},
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchAdvanced results length = %d, want %d", len(results), 1)
	}
	if results[0].Episode.Topics != "go,programming" {
		t.Errorf("expected go tag, got %s", results[0].Episode.Topics)
	}
}

func TestSQLiteStore_SearchAdvanced_MinScore(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "The quick brown fox jumps over the lazy dog"))
	_ = store.Add(ctx, MustEpisode("s1", "assistant", "Something completely unrelated"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:      "fox",
		MinScore:   0.01,
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchAdvanced results length = %d, want %d", len(results), 1)
	}
}

func TestSQLiteStore_SearchAdvanced_SemanticWeight(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "go programming language tutorial")
	ep1.Importance = 0.8
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "go programming basics for beginners")
	ep2.Importance = 0.2
	_ = store.Add(ctx, ep2)

	resultsSemantic, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:          "go programming",
		UseSemantic:    true,
		SemanticWeight: 1.0,
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(resultsSemantic) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(resultsSemantic))
	}

	resultsKeyword, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:          "go programming",
		UseSemantic:    false,
		SemanticWeight: 0.0,
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}

	if resultsSemantic[0].SemanticScore <= 0 {
		t.Errorf("SemanticScore should be > 0 when UseSemantic=true, got %f", resultsSemantic[0].SemanticScore)
	}
	if resultsKeyword[0].SemanticScore != 0 {
		t.Errorf("SemanticScore should be 0 when UseSemantic=false, got %f", resultsKeyword[0].SemanticScore)
	}
}

func TestSQLiteStore_SearchAdvanced_SessionFilter(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("session-a", "user", "Go programming language"))
	_ = store.Add(ctx, MustEpisode("session-b", "user", "Python programming language"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:      "programming",
		SessionID:  "session-a",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("SearchAdvanced results length = %d, want %d", len(results), 1)
	}
	if results[0].Episode.SessionID != "session-a" {
		t.Errorf("SessionID = %q, want %q", results[0].Episode.SessionID, "session-a")
	}
}

func TestSQLiteStore_SearchAdvanced_EmptyQuery(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	_ = store.Add(ctx, MustEpisode("s1", "user", "Hello world"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:      "",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchAdvanced results length = %d, want %d", len(results), 0)
	}
}

func TestSQLiteStore_SearchAdvanced_CombinedScoreSorting(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Go is a great programming language for building fast applications")
	ep1.Importance = 0.9
	_ = store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Go programming basics")
	ep2.Importance = 0.1
	_ = store.Add(ctx, ep2)

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:          "Go programming",
		UseSemantic:    true,
		SemanticWeight: 0.5,
		MaxResults:     10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}

	if results[0].CombinedScore < results[1].CombinedScore {
		t.Errorf("results should be sorted by CombinedScore DESC, got %f < %f", results[0].CombinedScore, results[1].CombinedScore)
	}
}
