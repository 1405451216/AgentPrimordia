package orchestration

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteMapReduce_Basic(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		Name:    "test-mr",
		Mode:    ParallelMode,
		Timeout: 30 * time.Second,
	})
	mapper := func(ctx context.Context, item any) (any, error) {
		n := item.(int)
		return n * n, nil
	}
	reducer := func(results []any) (any, error) {
		sum := 0
		for _, r := range results {
			sum += r.(int)
		}
		return sum, nil
	}
	input := []any{1, 2, 3, 4, 5}
	result, err := o.ExecuteMapReduce(context.Background(), MapReduceConfig{
		MapperCount: 2,
		Mapper:      mapper,
		Reducer:     reducer,
	}, input)
	if err != nil {
		t.Fatalf("ExecuteMapReduce error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", result.Status)
	}
	output, ok := result.FinalOutput["result"]
	if !ok {
		t.Fatal("expected result in FinalOutput")
	}
	if output.(int) != 55 {
		t.Errorf("expected 55, got %v", output)
	}
	if result.Metrics.TotalSteps != 5 {
		t.Errorf("expected 5 map steps, got %d", result.Metrics.TotalSteps)
	}
}

func TestExecuteMapReduce_EmptyInput(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{Name: "test-mr-empty"})
	reducer := func(results []any) (any, error) { return 0, nil }
	result, err := o.ExecuteMapReduce(context.Background(), MapReduceConfig{
		MapperCount: 1,
		Mapper:      func(ctx context.Context, item any) (any, error) { return item, nil },
		Reducer:     reducer,
	}, []any{})
	if err != nil {
		t.Fatalf("ExecuteMapReduce error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", result.Status)
	}
}

func TestExecuteMapReduce_MapperError(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{Name: "test-mr-err"})
	mapper := func(ctx context.Context, item any) (any, error) {
		if item.(int) == 3 {
			return nil, errors.New("boom")
		}
		return item, nil
	}
	reducer := func(results []any) (any, error) { return len(results), nil }
	input := []any{1, 2, 3, 4, 5}
	result, _ := o.ExecuteMapReduce(context.Background(), MapReduceConfig{
		MapperCount: 2,
		Mapper:      mapper,
		Reducer:     reducer,
	}, input)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestExecuteMapReduce_Timeout(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{Name: "test-mr-timeout"})
	mapper := func(ctx context.Context, item any) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			return item, nil
		}
	}
	reducer := func(results []any) (any, error) { return results, nil }
	input := []any{1, 2, 3}
	_, err := o.ExecuteMapReduce(context.Background(), MapReduceConfig{
		MapperCount: 3,
		Mapper:      mapper,
		Reducer:     reducer,
		Timeout:     200 * time.Millisecond,
	}, input)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExecuteScatterGather_Basic(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{
		Name:    "test-sg",
		Timeout: 30 * time.Second,
	})
	var callCount int32
	scatter := func(ctx context.Context, item any) (any, error) {
		atomic.AddInt32(&callCount, 1)
		return item.(string) + "_processed", nil
	}
	gatherer := func(results []any) (any, error) {
		combined := ""
		for _, r := range results {
			combined += r.(string)
		}
		return combined, nil
	}
	tasks := []any{"a", "b", "c"}
	result, err := o.ExecuteScatterGather(context.Background(), ScatterGatherConfig{
		Gatherer:       gatherer,
		Scatterer:      scatter,
		MaxGatherers:   3,
		PartialResults: true,
	}, tasks)
	if err != nil {
		t.Fatalf("ExecuteScatterGather error: %v", err)
	}
	if result.Status != StatusCompleted {
		t.Errorf("expected StatusCompleted, got %s", result.Status)
	}
	output, ok := result.FinalOutput["result"]
	if !ok {
		t.Fatal("expected result in FinalOutput")
	}
	if output.(string) == "" {
		t.Error("expected non-empty output")
	}
	if atomic.LoadInt32(&callCount) != 3 {
		t.Errorf("expected 3 scatter calls, got %d", callCount)
	}
}

func TestExecuteScatterGather_PartialResults(t *testing.T) {
	o := NewOrchestrator(OrchestratorConfig{Name: "test-sg-partial"})
	var callCount int32
	scatter := func(ctx context.Context, item any) (any, error) {
		idx := item.(int)
		if idx == 2 {
			return nil, errors.New("fail")
		}
		atomic.AddInt32(&callCount, 1)
		return idx * 10, nil
	}
	gatherer := func(results []any) (any, error) {
		sum := 0
		for _, r := range results {
			sum += r.(int)
		}
		return sum, nil
	}
	tasks := []any{1, 2, 3}
	result, err := o.ExecuteScatterGather(context.Background(), ScatterGatherConfig{
		Gatherer:       gatherer,
		Scatterer:      scatter,
		MaxGatherers:   2,
		PartialResults: true,
		Timeout:        5 * time.Second,
	}, tasks)
	if err != nil {
		t.Fatalf("ExecuteScatterGather error: %v", err)
	}
	output, ok := result.FinalOutput["result"]
	if !ok {
		t.Fatal("expected partial results")
	}
	if output.(int) != 40 {
		t.Errorf("expected 40 (10+30), got %d", output.(int))
	}
}
