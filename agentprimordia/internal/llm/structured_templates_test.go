package llm

import (
	"testing"
)

func TestSentimentSchema(t *testing.T) {
	schema := SentimentSchema()
	if schema.Name != "sentiment" {
		t.Errorf("Name = %q, want sentiment", schema.Name)
	}
	props, ok := schema.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not found")
	}
	if _, has := props["sentiment"]; !has {
		t.Error("sentiment property missing")
	}
	if _, has := props["score"]; !has {
		t.Error("score property missing")
	}
	if _, has := props["confidence"]; !has {
		t.Error("confidence property missing")
	}

	sentimentProp := props["sentiment"].(map[string]any)
	if enums, ok := sentimentProp["enum"].([]string); ok {
		found := false
		for _, e := range enums {
			if e == "positive" {
				found = true
				break
			}
		}
		if !found {
			t.Error("sentiment enum should contain 'positive'")
		}
	}
}

func TestNERSchema(t *testing.T) {
	schema := NERSchema()
	if schema.Name != "ner" {
		t.Errorf("Name = %q, want ner", schema.Name)
	}
	props, ok := schema.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties not found")
	}
	entitiesProp, has := props["entities"]
	if !has {
		t.Fatal("entities property missing")
	}
	entitiesMap := entitiesProp.(map[string]any)
	itemsMap, ok := entitiesMap["items"].(map[string]any)
	if !ok {
		t.Fatal("entities items not found")
	}
	itemProps := itemsMap["properties"].(map[string]any)
	if _, has := itemProps["text"]; !has {
		t.Error("entity text property missing")
	}
	if _, has := itemProps["type"]; !has {
		t.Error("entity type property missing")
	}
}

func TestClassificationSchema(t *testing.T) {
	schema := ClassificationSchema()
	if schema.Name != "classification" {
		t.Errorf("Name = %q, want classification", schema.Name)
	}
	props := schema.Schema["properties"].(map[string]any)
	if _, has := props["category"]; !has {
		t.Error("category property missing")
	}
	if _, has := props["subcategory"]; !has {
		t.Error("subcategory property missing")
	}
	if _, has := props["confidence"]; !has {
		t.Error("confidence property missing")
	}
}

func TestSummarySchema(t *testing.T) {
	schema := SummarySchema()
	if schema.Name != "summary" {
		t.Errorf("Name = %q, want summary", schema.Name)
	}
	props := schema.Schema["properties"].(map[string]any)
	if _, has := props["summary"]; !has {
		t.Error("summary property missing")
	}
	if _, has := props["key_points"]; !has {
		t.Error("key_points property missing")
	}
	if _, has := props["word_count"]; !has {
		t.Error("word_count property missing")
	}
}

func TestMultiLabelClassificationSchema(t *testing.T) {
	schema := MultiLabelClassificationSchema()
	if schema.Name != "multi_label_classification" {
		t.Errorf("Name = %q, want multi_label_classification", schema.Name)
	}
	props := schema.Schema["properties"].(map[string]any)
	if _, has := props["labels"]; !has {
		t.Error("labels property missing")
	}
}

func TestExtractiveSummarySchema(t *testing.T) {
	schema := ExtractiveSummarySchema()
	if schema.Name != "extractive_summary" {
		t.Errorf("Name = %q, want extractive_summary", schema.Name)
	}
	props := schema.Schema["properties"].(map[string]any)
	if _, has := props["compression_ratio"]; !has {
		t.Error("compression_ratio property missing")
	}
}

func TestSentimentDetailSchema(t *testing.T) {
	schema := SentimentDetailSchema()
	if schema.Name != "sentiment_detail" {
		t.Errorf("Name = %q, want sentiment_detail", schema.Name)
	}
	props := schema.Schema["properties"].(map[string]any)
	if _, has := props["aspects"]; !has {
		t.Error("aspects property missing")
	}
}

func TestTemplateSchemasAreValid(t *testing.T) {
	templates := map[string]*SchemaDef{
		"sentiment":                  SentimentSchema(),
		"sentiment_detail":           SentimentDetailSchema(),
		"ner":                        NERSchema(),
		"classification":             ClassificationSchema(),
		"multi_label_classification": MultiLabelClassificationSchema(),
		"summary":                    SummarySchema(),
		"extractive_summary":         ExtractiveSummarySchema(),
	}

	for name, schema := range templates {
		t.Run(name, func(t *testing.T) {
			if schema.Name == "" {
				t.Error("schema name should not be empty")
			}
			if schema.Schema == nil {
				t.Error("schema.Schema should not be nil")
			}
			if schema.Schema["type"] != "object" {
				t.Errorf("schema type = %v, want object", schema.Schema["type"])
			}
			if _, has := schema.Schema["properties"]; !has {
				t.Error("schema should have properties")
			}
		})
	}
}
