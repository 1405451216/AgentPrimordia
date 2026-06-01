package memory

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestNewSQLiteStore_InMemory(t *testing.T) {
	store, err := WithInMemory()
	if err != nil {
		t.Fatalf("WithInMemory() error = %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestNewSQLiteStore_FileBased(t *testing.T) {
	dir := t.TempDir()
	dbPath := dir + "/test.db"

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore() error = %v", err)
	}
	defer store.Close()

	if store == nil {
		t.Fatal("expected non-nil store")
	}
}

func TestAdd_Episode(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	episode := MustEpisode("session-1", "user", "Hello, how are you?")

	err := store.Add(ctx, episode)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if episode.ID == "" {
		t.Fatal("expected episode ID to be set")
	}

	got, err := store.Get(ctx, episode.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Content != "Hello, how are you?" {
		t.Errorf("Content = %q, want %q", got.Content, "Hello, how are you?")
	}
	if got.Role != "user" {
		t.Errorf("Role = %q, want %q", got.Role, "user")
	}
	if got.SessionID != "session-1" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "session-1")
	}
}

func TestAdd_MultipleEpisodes(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	episodes := []*Episode{
		MustEpisode("session-1", "user", "First message"),
		MustEpisode("session-1", "assistant", "Second message"),
		MustEpisode("session-2", "user", "Third message"),
	}

	for _, ep := range episodes {
		err := store.Add(ctx, ep)
		if err != nil {
			t.Fatalf("Add() error = %v", err)
		}
	}

	count, err := store.Count(ctx, "")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 3 {
		t.Errorf("Count = %d, want %d", count, 3)
	}
}

func TestGet_ByID(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	episode := MustEpisode("session-1", "assistant", "This is a response")

	err := store.Add(ctx, episode)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	got, err := store.Get(ctx, episode.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.ID != episode.ID {
		t.Errorf("ID = %q, want %q", got.ID, episode.ID)
	}
	if got.Content != episode.Content {
		t.Errorf("Content = %q, want %q", got.Content, episode.Content)
	}
}

func TestGet_NotFound(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	_, err := store.Get(ctx, "non-existent-id")
	if err == nil {
		t.Fatal("expected error for non-existent ID")
	}
}

func TestDelete_Episode(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	episode := MustEpisode("session-1", "user", "To be deleted")

	err := store.Add(ctx, episode)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	err = store.Delete(ctx, episode.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	_, err = store.Get(ctx, episode.ID)
	if err == nil {
		t.Fatal("expected error after deletion")
	}
}

func TestDelete_NotFound(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	err := store.Delete(ctx, "non-existent-id")
	if err != nil {
		t.Fatalf("Delete() should not error for non-existent ID, got %v", err)
	}
}

func TestCount_All(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		ep := MustEpisode("session-1", "user", "Message")
		store.Add(ctx, ep)
	}

	count, err := store.Count(ctx, "")
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 5 {
		t.Errorf("Count = %d, want %d", count, 5)
	}
}

func TestCount_BySession(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		ep := MustEpisode("session-a", "user", "Message A")
		store.Add(ctx, ep)
	}
	for i := 0; i < 7; i++ {
		ep := MustEpisode("session-b", "user", "Message B")
		store.Add(ctx, ep)
	}

	countA, err := store.Count(ctx, "session-a")
	if err != nil {
		t.Fatalf("Count(session-a) error = %v", err)
	}
	if countA != 3 {
		t.Errorf("Count(session-a) = %d, want %d", countA, 3)
	}

	countB, err := store.Count(ctx, "session-b")
	if err != nil {
		t.Fatalf("Count(session-b) error = %v", err)
	}
	if countB != 7 {
		t.Errorf("Count(session-b) = %d, want %d", countB, 7)
	}

	countAll, _ := store.Count(ctx, "")
	if countAll != 10 {
		t.Errorf("Count(all) = %d, want %d", countAll, 10)
	}
}

func TestSearch_BasicMatch(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "The quick brown fox jumps over the lazy dog"))
	store.Add(ctx, MustEpisode("s1", "assistant", "I like programming in Go"))

	results, err := store.Search(ctx, "fox", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("Search results length = %d, want %d", len(results), 1)
	}
}

func TestSearch_NoMatch(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Hello world"))

	results, err := store.Search(ctx, "nonexistentterm", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search results length = %d, want %d", len(results), 0)
	}
}

func TestSearch_WithSessionFilter(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("session-a", "user", "Go programming language"))
	store.Add(ctx, MustEpisode("session-b", "user", "Python programming language"))

	results, err := store.Search(ctx, "programming", &SearchOptions{
		SessionID: "session-a",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search results length = %d, want %d", len(results), 1)
	}
	if results[0].SessionID != "session-a" {
		t.Errorf("SessionID = %q, want %q", results[0].SessionID, "session-a")
	}
}

func TestSearch_WithLimit(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 10; i++ {
		store.Add(ctx, MustEpisode("s1", "user", "test message about search"))
	}

	results, err := store.Search(ctx, "search", &SearchOptions{Limit: 3})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) > 3 {
		t.Errorf("Search results length = %d, want <= %d", len(results), 3)
	}
}

func TestSearch_WithRoleFilter(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "What is Go?"))
	store.Add(ctx, MustEpisode("s1", "assistant", "Go is a programming language"))

	results, err := store.Search(ctx, "Go", &SearchOptions{
		Limit:      10,
		RoleFilter: "assistant",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search results length = %d, want %d", len(results), 1)
	}
	if results[0].Role != "assistant" {
		t.Errorf("Role = %q, want %q", results[0].Role, "assistant")
	}
}

func TestSearch_SummarySearch(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Long content here")
	ep.Summary = "Short summary about AI"
	store.Add(ctx, ep)

	results, err := store.Search(ctx, "AI", &SearchOptions{Limit: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search results length = %d, want %d (should find in summary)", len(results), 1)
	}
}

func TestList_Pagination(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	for i := 0; i < 15; i++ {
		store.Add(ctx, MustEpisode("s1", "user", "message"))
	}

	page1, err := store.List(ctx, &ListOptions{Limit: 5, Offset: 0})
	if err != nil {
		t.Fatalf("List(page1) error = %v", err)
	}
	if len(page1) != 5 {
		t.Errorf("Page1 length = %d, want %d", len(page1), 5)
	}

	page2, err := store.List(ctx, &ListOptions{Limit: 5, Offset: 5})
	if err != nil {
		t.Fatalf("List(page2) error = %v", err)
	}
	if len(page2) != 5 {
		t.Errorf("Page2 length = %d, want %d", len(page2), 5)
	}

	page3, err := store.List(ctx, &ListOptions{Limit: 5, Offset: 10})
	if err != nil {
		t.Fatalf("List(page3) error = %v", err)
	}
	if len(page3) != 5 {
		t.Errorf("Page3 length = %d, want %d", len(page3), 5)
	}
}

func TestList_Ordering(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	var firstID string
	for i := 0; i < 5; i++ {
		ep := MustEpisode("s1", "user", "message")
		store.Add(ctx, ep)
		if i == 0 {
			firstID = ep.ID
		}
	}
	time.Sleep(10 * time.Millisecond)

	descResults, err := store.List(ctx, &ListOptions{
		Limit:     10,
		OrderBy:   "created_at",
		Ascending: false,
	})
	if err != nil {
		t.Fatalf("List(desc) error = %v", err)
	}
	if descResults[0].ID == firstID {
		t.Error("descending order should have newest first")
	}

	ascResults, err := store.List(ctx, &ListOptions{
		Limit:     10,
		OrderBy:   "created_at",
		Ascending: true,
	})
	if err != nil {
		t.Fatalf("List(asc) error = %v", err)
	}
	if ascResults[0].ID != firstID {
		t.Error("ascending order should have oldest first")
	}
}

func TestList_BySession(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("session-x", "user", "msg x1"))
	store.Add(ctx, MustEpisode("session-y", "user", "msg y1"))
	store.Add(ctx, MustEpisode("session-x", "user", "msg x2"))

	results, err := store.List(ctx, &ListOptions{
		SessionID: "session-x",
		Limit:     10,
	})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("List by session length = %d, want %d", len(results), 2)
	}
	for _, r := range results {
		if r.SessionID != "session-x" {
			t.Errorf("unexpected SessionID = %q", r.SessionID)
		}
	}
}

func TestEmptyContent(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	_, err := NewEpisode("s1", "user", "")
	if err == nil {
		t.Fatal("expected error for empty content")
	}

	ep := MustEpisode("s1", "user", "test content")
	err = store.Add(context.Background(), ep)
	if err != nil {
		t.Fatalf("unexpected error adding valid episode: %v", err)
	}
}

func TestSpecialCharacters(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	specialContent := "'; DROP TABLE episodes; --"
	ep := MustEpisode("s1", "user", specialContent)

	err := store.Add(ctx, ep)
	if err != nil {
		t.Fatalf("Add() with special chars error = %v", err)
	}

	got, err := store.Get(ctx, ep.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Content != specialContent {
		t.Errorf("Content = %q, want %q", got.Content, specialContent)
	}
}

func TestConcurrentAccess(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	var wg sync.WaitGroup
	errors := make(chan error, 20)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ep := MustEpisode("concurrent-session", "user", "concurrent msg")
			if err := store.Add(ctx, ep); err != nil {
				errors <- err
			}
			if _, err := store.Search(ctx, "concurrent", &SearchOptions{Limit: 10}); err != nil {
				errors <- err
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent access error: %v", err)
	}
}

func TestClose_DoubleClose(t *testing.T) {
	store, _ := WithInMemory()

	err := store.Close()
	if err != nil {
		t.Fatalf("first Close() error = %v", err)
	}

	err = store.Close()
	if err != nil {
		t.Fatalf("second Close() should not error, got %v", err)
	}
}

func TestEnhanced_UpdateSummary(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Hello world")
	store.Add(ctx, ep)

	err := store.UpdateSummary(ctx, ep.ID, "Greeting", "greeting,hello")
	if err != nil {
		t.Fatalf("UpdateSummary() error = %v", err)
	}

	got, _ := store.Get(ctx, ep.ID)
	if got.Summary != "Greeting" {
		t.Errorf("Summary = %q, want %q", got.Summary, "Greeting")
	}
	if got.Topics != "greeting,hello" {
		t.Errorf("Topics = %q, want %q", got.Topics, "greeting,hello")
	}
}

func TestEnhanced_SetImportance(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Important message")
	store.Add(ctx, ep)

	err := store.SetImportance(ctx, ep.ID, 0.8)
	if err != nil {
		t.Fatalf("SetImportance() error = %v", err)
	}

	got, _ := store.Get(ctx, ep.ID)
	if got.Importance != 0.8 {
		t.Errorf("Importance = %f, want %f", got.Importance, 0.8)
	}
}

func TestEnhanced_SearchByTag(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Message 1")
	ep1.Topics = "go,programming"
	store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Message 2")
	ep2.Topics = "python,data"
	store.Add(ctx, ep2)

	results, err := store.SearchByTag(ctx, "go", nil)
	if err != nil {
		t.Fatalf("SearchByTag() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchByTag results length = %d, want %d", len(results), 1)
	}
}

func TestEnhanced_SearchByTag_NoResults(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Message")
	ep.Topics = "go"
	store.Add(ctx, ep)

	results, err := store.SearchByTag(ctx, "nonexistent", nil)
	if err != nil {
		t.Fatalf("SearchByTag() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("SearchByTag results length = %d, want %d", len(results), 0)
	}
}

func TestEnhanced_GetImportant(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Low importance")
	ep1.Importance = 0.2
	store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "High importance")
	ep2.Importance = 0.9
	store.Add(ctx, ep2)

	ep3 := MustEpisode("s1", "user", "Medium importance")
	ep3.Importance = 0.5
	store.Add(ctx, ep3)

	results, err := store.GetImportant(ctx, 0.5, 10)
	if err != nil {
		t.Fatalf("GetImportant() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("GetImportant results length = %d, want %d", len(results), 2)
	}
	if results[0].Importance < results[1].Importance {
		t.Error("results should be sorted by importance DESC")
	}
}

func TestEnhanced_GetImportant_Empty(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Low importance")
	ep.Importance = 0.1
	store.Add(ctx, ep)

	results, err := store.GetImportant(ctx, 0.9, 10)
	if err != nil {
		t.Fatalf("GetImportant() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("GetImportant results length = %d, want %d", len(results), 0)
	}
}

func TestEnhanced_GetTimeline(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Today message"))

	results, err := store.GetTimeline(ctx, 7)
	if err != nil {
		t.Fatalf("GetTimeline() error = %v", err)
	}
	if len(results) == 0 {
		t.Error("expected at least one day in timeline")
	}
}

func TestEnhanced_CleanupExpired(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Recent message"))

	deleted, err := store.CleanupExpired(ctx, 30)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupExpired deleted = %d, want %d", deleted, 0)
	}
}

func TestEnhanced_CleanupExpired_None(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Message"))

	deleted, err := store.CleanupExpired(ctx, 0)
	if err != nil {
		t.Fatalf("CleanupExpired() error = %v", err)
	}
	if deleted != 0 {
		t.Errorf("CleanupExpired deleted = %d, want %d", deleted, 0)
	}
}

func TestEnhanced_Stats(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Message 1"))
	store.Add(ctx, MustEpisode("s2", "user", "Message 2"))

	stats, err := store.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}
	if stats.TotalEpisodes != 2 {
		t.Errorf("TotalEpisodes = %d, want %d", stats.TotalEpisodes, 2)
	}
	if stats.TotalSessions != 2 {
		t.Errorf("TotalSessions = %d, want %d", stats.TotalSessions, 2)
	}
	if stats.AvgEpisodesPerSession != 1.0 {
		t.Errorf("AvgEpisodesPerSession = %f, want %f", stats.AvgEpisodesPerSession, 1.0)
	}
}

func TestEnhanced_Topics_Default(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep := MustEpisode("s1", "user", "Message")
	store.Add(ctx, ep)

	got, _ := store.Get(ctx, ep.ID)
	if got.Topics != "" {
		t.Errorf("Topics = %q, want empty string", got.Topics)
	}
}

func TestEnhanced_Importance_Range(t *testing.T) {
	ep := MustEpisode("s1", "user", "Message")
	ep.Importance = 1.5

	err := ep.Validate()
	if err == nil {
		t.Fatal("expected validation error for importance > 1")
	}
}

func TestEpisodeID_Monotonic(t *testing.T) {
	var ids []string
	for i := 0; i < 100; i++ {
		ids = append(ids, generateEpisodeID())
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("episode IDs not monotonic: %s <= %s at index %d", ids[i], ids[i-1], i)
		}
	}
}

func TestSearchAdvanced(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Go programming language"))
	store.Add(ctx, MustEpisode("s1", "assistant", "Python data science"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:    "Go",
		MaxResults: 10,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("SearchAdvanced results length = %d, want 1", len(results))
	}
	if results[0].Episode.Content != "Go programming language" {
		t.Errorf("wrong result: %s", results[0].Episode.Content)
	}
}

func TestSearchAdvanced_SemanticWeight(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Go programming language"))

	results, err := store.SearchAdvanced(ctx, SearchOptions{
		Query:          "Go",
		MaxResults:     10,
		UseSemantic:    true,
		SemanticWeight: 0.5,
	})
	if err != nil {
		t.Fatalf("SearchAdvanced() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestGetMemoriesByTag(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Message about Go")
	ep1.Topics = "go,programming"
	store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "Message about Python")
	ep2.Topics = "python,data"
	store.Add(ctx, ep2)

	results, err := store.GetMemoriesByTag(ctx, "go", 10)
	if err != nil {
		t.Fatalf("GetMemoriesByTag() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestGetMemoriesBySession(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("session-a", "user", "Msg A1"))
	store.Add(ctx, MustEpisode("session-b", "user", "Msg B1"))
	store.Add(ctx, MustEpisode("session-a", "user", "Msg A2"))

	results, err := store.GetMemoriesBySession(ctx, "session-a")
	if err != nil {
		t.Fatalf("GetMemoriesBySession() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.SessionID != "session-a" {
			t.Errorf("unexpected SessionID = %q", r.SessionID)
		}
	}
}

func TestGetImportantMemories(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	ep1 := MustEpisode("s1", "user", "Low")
	ep1.Importance = 0.2
	store.Add(ctx, ep1)

	ep2 := MustEpisode("s1", "user", "High")
	ep2.Importance = 0.9
	store.Add(ctx, ep2)

	results, err := store.GetImportantMemories(ctx, 0.5, 10)
	if err != nil {
		t.Fatalf("GetImportantMemories() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
}

func TestRecordToolUse(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	err := store.RecordToolUse(ctx, "session-1", "agent-1", "shell", "echo hello", "output")
	if err != nil {
		t.Fatalf("RecordToolUse() error = %v", err)
	}

	count, _ := store.Count(ctx, "")
	if count != 1 {
		t.Errorf("expected 1 episode, got %d", count)
	}

	results, _ := store.GetMemoriesBySession(ctx, "session-1")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Role != "tool_use" {
		t.Errorf("expected role 'tool_use', got %q", results[0].Role)
	}
	if results[0].Topics != "shell" {
		t.Errorf("expected topics 'shell', got %q", results[0].Topics)
	}
}

func TestClearAll_BySession(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("session-a", "user", "Msg A"))
	store.Add(ctx, MustEpisode("session-b", "user", "Msg B"))

	err := store.ClearAll(ctx, "session-a")
	if err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}

	countA, _ := store.Count(ctx, "session-a")
	if countA != 0 {
		t.Errorf("session-a should be empty, got %d", countA)
	}
	countB, _ := store.Count(ctx, "session-b")
	if countB != 1 {
		t.Errorf("session-b should have 1 episode, got %d", countB)
	}
}

func TestClearAll_EntireStore(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Msg 1"))
	store.Add(ctx, MustEpisode("s2", "user", "Msg 2"))

	err := store.ClearAll(ctx, "")
	if err != nil {
		t.Fatalf("ClearAll() error = %v", err)
	}

	count, _ := store.Count(ctx, "")
	if count != 0 {
		t.Errorf("store should be empty, got %d", count)
	}
}

func TestExportMemories_JSON(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Export test"))

	data, err := store.ExportMemories(ctx, "", "json")
	if err != nil {
		t.Fatalf("ExportMemories() error = %v", err)
	}
	if len(data) == 0 {
		t.Error("exported data should not be empty")
	}
}

func TestExportMemories_Markdown(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Export test"))

	data, err := store.ExportMemories(ctx, "", "markdown")
	if err != nil {
		t.Fatalf("ExportMemories() error = %v", err)
	}
	if len(data) == 0 {
		t.Error("exported data should not be empty")
	}
}

func TestImportMemories(t *testing.T) {
	store, _ := WithInMemory()
	defer store.Close()

	ctx := context.Background()
	store.Add(ctx, MustEpisode("s1", "user", "Original"))

	// 导出再导入
	data, _ := store.ExportMemories(ctx, "", "json")
	store.ClearAll(ctx, "")

	count, err := store.ImportMemories(ctx, data, "json")
	if err != nil {
		t.Fatalf("ImportMemories() error = %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 imported, got %d", count)
	}

	total, _ := store.Count(ctx, "")
	if total != 1 {
		t.Errorf("expected 1 episode after import, got %d", total)
	}
}
