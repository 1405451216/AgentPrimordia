package prompt

import (
	"strings"
	"testing"
)

func TestFewShotTemplate_Basic(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate:  "{{.examples}}用户输入: {{.user_input}}",
		ExampleFormat: "输入: {{.Input}}\n输出: {{.Output}}\n",
		MaxExamples:   3,
	})
	fst.AddExample("hello", "你好")
	fst.AddExample("bye", "再见")

	result, err := fst.Render("thanks")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, "你好") {
		t.Error("expected example output in result")
	}
	if !strings.Contains(result, "thanks") {
		t.Error("expected user input in result")
	}
}

func TestFewShotTemplate_WithMetadata(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.examples}}{{.user_input}}",
	})
	fst.AddExampleWithMetadata("q1", "a1", map[string]any{"category": "test"})

	examples := fst.GetExamples()
	if len(examples) != 1 {
		t.Fatalf("expected 1 example, got %d", len(examples))
	}
	if examples[0].Metadata["category"] != "test" {
		t.Error("expected metadata category=test")
	}
}

func TestFewShotTemplate_AddExamples(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.examples}}{{.user_input}}",
	})
	fst.AddExamples([]Example{
		{Input: "q1", Output: "a1"},
		{Input: "q2", Output: "a2"},
	})
	if len(fst.GetExamples()) != 2 {
		t.Errorf("expected 2 examples, got %d", len(fst.GetExamples()))
	}
}

func TestFewShotTemplate_ClearExamples(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.examples}}{{.user_input}}",
	})
	fst.AddExample("q1", "a1")
	fst.ClearExamples()
	if len(fst.GetExamples()) != 0 {
		t.Errorf("expected 0 examples after clear, got %d", len(fst.GetExamples()))
	}
}

func TestFewShotTemplate_WithVar(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.prefix}} {{.examples}}{{.user_input}}",
	})
	fst.WithVar("prefix", "Context:")
	result, err := fst.Render("test")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, "Context:") {
		t.Error("expected prefix in result")
	}
}

func TestFewShotTemplate_MaxExamples(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate:  "{{.examples}}{{.user_input}}",
		ExampleFormat: "输入: {{.Input}}\n输出: {{.Output}}\n",
		MaxExamples:   2,
	})
	fst.AddExample("q1", "a1")
	fst.AddExample("q2", "a2")
	fst.AddExample("q3", "a3")
	fst.AddExample("q4", "a4")

	result, err := fst.Render("test")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}

	count := 0
	for _, a := range []string{"a1", "a2", "a3", "a4"} {
		if strings.Contains(result, a) {
			count++
		}
	}
	if count > 2 {
		t.Errorf("should not contain more than max examples, got %d", count)
	}
}

func TestFewShotTemplate_EmptyExamples(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.examples}}Input: {{.user_input}}",
	})
	result, err := fst.Render("hello")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, "hello") {
		t.Error("expected user input in result")
	}
}

func TestLengthBasedSelector(t *testing.T) {
	selector := &LengthBasedSelector{}
	examples := []*Example{
		{Input: "short", Output: "a"},
		{Input: "a bit longer input text", Output: "b"},
		{Input: "this is a very very long input text for testing", Output: "c"},
	}

	selected, err := selector.SelectExamples("medium length input", examples)
	if err != nil {
		t.Fatalf("SelectExamples error: %v", err)
	}
	if len(selected) != 3 {
		t.Errorf("expected 3 results, got %d", len(selected))
	}
}

func TestSimilaritySelector(t *testing.T) {
	selector := &SimilaritySelector{}
	examples := []*Example{
		{Input: "go programming language", Output: "a"},
		{Input: "python data science", Output: "b"},
		{Input: "go web development", Output: "c"},
	}

	selected, err := selector.SelectExamples("go programming tutorial", examples)
	if err != nil {
		t.Fatalf("SelectExamples error: %v", err)
	}
	if len(selected) == 0 {
		t.Error("expected at least one result")
	}
	if selected[0].Input != "go programming language" {
		t.Errorf("expected most similar first, got '%s'", selected[0].Input)
	}
}

func TestRandomSelector(t *testing.T) {
	selector := &RandomSelector{Seed: 42}
	examples := []*Example{
		{Input: "q1", Output: "a1"},
		{Input: "q2", Output: "a2"},
		{Input: "q3", Output: "a3"},
	}

	selected, err := selector.SelectExamples("", examples)
	if err != nil {
		t.Fatalf("SelectExamples error: %v", err)
	}
	if len(selected) != 3 {
		t.Errorf("expected 3 results, got %d", len(selected))
	}
}

func TestFewShotTemplate_WithSelector(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate:  "{{.examples}}{{.user_input}}",
		ExampleFormat: "输入: {{.Input}}\n输出: {{.Output}}\n",
		MaxExamples:   1,
	})
	fst.AddExample("go programming", "Go is great")
	fst.AddExample("python data", "Python is versatile")
	fst.SetSelector(&SimilaritySelector{})

	result, err := fst.Render("go tutorial")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, "Go is great") {
		t.Error("expected most similar example selected")
	}
}

func TestFewShotTemplate_DefaultConfig(t *testing.T) {
	fst := NewFewShotTemplate(FewShotConfig{
		BaseTemplate: "{{.examples}}{{.user_input}}",
	})
	fst.AddExample("q1", "a1")
	result, err := fst.Render("test")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(result, "示例") {
		t.Error("expected default prefix with '示例'")
	}
}
