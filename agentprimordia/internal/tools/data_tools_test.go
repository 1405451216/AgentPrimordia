package tools

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCSVTool_Read(t *testing.T) {
	csvTool := NewCSVTool()

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "test.csv")
	csvContent := "name,age,city\nAlice,30,New York\nBob,25,San Francisco\nCharlie,35,Chicago\n"
	os.WriteFile(csvPath, []byte(csvContent), 0644)

	input := map[string]any{"action": "read", "file_path": csvPath}
	inputBytes, _ := json.Marshal(input)

	result, err := csvTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var data []map[string]any
	json.Unmarshal([]byte(result.Content), &data)

	if len(data) != 3 {
		t.Errorf("expected 3 rows, got %d", len(data))
	}
	if data[0]["name"] != "Alice" {
		t.Errorf("expected first name 'Alice', got '%s'", data[0]["name"])
	}
	t.Logf("✅ CSV Read: %d rows loaded", len(data))
}

func TestCSVTool_Filter(t *testing.T) {
	csvTool := NewCSVTool()

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "filter.csv")
	csvContent := "product,category,price\nLaptop,Electronics,999\nBook,Education,29\nPhone,Electronics,699\nPen,Stationery,2\n"
	os.WriteFile(csvPath, []byte(csvContent), 0644)

	input := map[string]any{"action": "filter", "file_path": csvPath, "filter_column": "category", "filter_value": "Electronics"}
	inputBytes, _ := json.Marshal(input)

	result, err := csvTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var filteredData []map[string]any
	json.Unmarshal([]byte(result.Content), &filteredData)

	if len(filteredData) != 2 {
		t.Errorf("expected 2 Electronics items, got %d", len(filteredData))
	}
	t.Logf("✅ CSV Filter: %d items match criteria", len(filteredData))
}

func TestCSVTool_Aggregate(t *testing.T) {
	csvTool := NewCSVTool()

	tmpDir := t.TempDir()
	csvPath := filepath.Join(tmpDir, "agg.csv")
	csvContent := "item,quantity,price\nA,10,100\nB,20,200\nC,15,150\n"
	os.WriteFile(csvPath, []byte(csvContent), 0644)

	input := map[string]any{"action": "aggregate", "file_path": csvPath, "aggregate_column": "price", "aggregate_func": "sum"}
	inputBytes, _ := json.Marshal(input)

	result, err := csvTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var aggResult map[string]any
	json.Unmarshal([]byte(result.Content), &aggResult)

	sum := aggResult["value"].(float64)
	if sum != 450 {
		t.Errorf("expected sum 450.0, got %.1f", sum)
	}
	t.Logf("✅ CSV Aggregate: sum = %.1f", sum)
}

func TestCSVTool_Write(t *testing.T) {
	csvTool := NewCSVTool()

	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "output.csv")

	data := []map[string]any{{"name": "X", "value": 100}, {"name": "Y", "value": 200}}

	input := map[string]any{"action": "write", "file_path": outputPath, "data": data}
	inputBytes, _ := json.Marshal(input)

	result, err := csvTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !strings.Contains(result.Content, "Successfully wrote") {
		t.Error("expected success message")
	}

	content, _ := os.ReadFile(outputPath)
	if !strings.Contains(string(content), "X") || !strings.Contains(string(content), "Y") {
		t.Error("output file content mismatch")
	}
	t.Logf("✅ CSV Write: %s", result.Content)
}

func TestJSONTool_Parse(t *testing.T) {
	jsonTool := NewJSONTool()

	testJSON := `{"users": [{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}]}`
	input := map[string]any{"action": "parse", "data": testJSON}
	inputBytes, _ := json.Marshal(input)

	result, err := jsonTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	var parsed any
	json.Unmarshal([]byte(result.Content), &parsed)
	if parsed == nil {
		t.Error("expected parsed JSON object")
	}
	t.Logf("✅ JSON Parse: successfully parsed complex nested structure")
}

func TestJSONTool_Query(t *testing.T) {
	jsonTool := NewJSONTool()

	testJSON := `{"company": "Acme", "employees": [{"name": "Alice", "role": "Engineer"}, {"name": "Bob", "role": "Manager"}]}`
	input := map[string]any{"action": "query", "data": testJSON, "path": "employees.0.name"}
	inputBytes, _ := json.Marshal(input)

	result, err := jsonTool.Execute(context.Background(), inputBytes)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if !strings.Contains(result.Content, "Alice") {
		t.Errorf("expected 'Alice' in result, got: %s", result.Content)
	}
	t.Logf("✅ JSON Query: path query successful - %s", strings.TrimSpace(result.Content))
}

func TestJSONTool_Validate(t *testing.T) {
	jsonTool := NewJSONTool()

	validJSON := `{"name": "test", "count": 42}`
	invalidJSON := `{invalid json`

	inputValid := map[string]any{"action": "validate", "data": validJSON}
	inputBytes, _ := json.Marshal(inputValid)
	result, _ := jsonTool.Execute(context.Background(), inputBytes)
	if !strings.Contains(result.Content, `"valid": true`) {
		t.Error("expected valid JSON to pass validation")
	}

	inputInvalid := map[string]any{"action": "validate", "data": invalidJSON}
	inputBytes2, _ := json.Marshal(inputInvalid)
	result2, _ := jsonTool.Execute(context.Background(), inputBytes2)
	if strings.Contains(result2.Content, `"valid": true`) {
		t.Error("expected invalid JSON to fail validation")
	}
	t.Logf("✅ JSON Validate: correctly identified valid/invalid JSON")
}

func TestSQLiteTool_BasicOperations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	sqliteTool, err := NewSQLiteTool(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteTool error: %v", err)
	}
	defer sqliteTool.Close()

	_, err = sqliteTool.db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, email TEXT)")
	if err != nil {
		t.Fatalf("Create table error: %v", err)
	}

	_, err = sqliteTool.db.Exec("INSERT INTO users (name, email) VALUES ('Alice', 'alice@example.com'), ('Bob', 'bob@example.com')")
	if err != nil {
		t.Fatalf("Insert error: %v", err)
	}

	queryInput := map[string]any{"action": "query", "sql": "SELECT * FROM users"}
	queryBytes, _ := json.Marshal(queryInput)
	queryResult, err := sqliteTool.Execute(context.Background(), queryBytes)
	if err != nil {
		t.Fatalf("Query error: %v", err)
	}

	var users []map[string]any
	json.Unmarshal([]byte(queryResult.Content), &users)

	if len(users) != 2 {
		t.Errorf("expected 2 users, got %d", len(users))
	}

	tablesInput := map[string]any{"action": "tables"}
	tablesBytes, _ := json.Marshal(tablesInput)
	tablesResult, _ := sqliteTool.Execute(context.Background(), tablesBytes)
	if !strings.Contains(tablesResult.Content, "users") {
		t.Error("'users' table not found in tables list")
	}
	t.Logf("✅ SQLite: created table, inserted %d records, queried successfully", len(users))
}

func TestSQLiteTool_SQLSafetyCheck(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "safety.db")
	sqliteTool, err := NewSQLiteTool(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteTool error: %v", err)
	}
	defer sqliteTool.Close()

	dangerousSQL := []struct {
		name string
		sql  string
	}{
		{"DROP TABLE", "DROP TABLE users"},
		{"DELETE", "DELETE FROM users"},
		{"UPDATE", "UPDATE users SET name='hacked'"},
		{"INSERT", "INSERT INTO users (name) VALUES ('evil')"},
		{"ALTER TABLE", "ALTER TABLE users ADD COLUMN password TEXT"},
		{"CREATE TABLE", "CREATE TABLE evil (id INT)"},
		{"TRUNCATE", "TRUNCATE users"},
		{"ATTACH", "ATTACH DATABASE '/etc/passwd' AS pwn"},
	}

	for _, tc := range dangerousSQL {
		t.Run("query/"+tc.name, func(t *testing.T) {
			input := map[string]any{"action": "query", "sql": tc.sql}
			inputBytes, _ := json.Marshal(input)
			result, err := sqliteTool.Execute(context.Background(), inputBytes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result.Content, "SQL safety check failed") {
				t.Errorf("expected safety check to block %s, got: %s", tc.name, result.Content)
			}
		})

		t.Run("execute/"+tc.name, func(t *testing.T) {
			input := map[string]any{"action": "execute", "sql": tc.sql}
			inputBytes, _ := json.Marshal(input)
			result, err := sqliteTool.Execute(context.Background(), inputBytes)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.Contains(result.Content, "SQL safety check failed") {
				t.Errorf("expected safety check to block %s, got: %s", tc.name, result.Content)
			}
		})
	}

	safeSQL := []struct {
		name string
		sql  string
	}{
		{"SELECT", "SELECT 1"},
		{"PRAGMA", "PRAGMA table_list"},
	}

	for _, tc := range safeSQL {
		t.Run("query/"+tc.name, func(t *testing.T) {
			input := map[string]any{"action": "query", "sql": tc.sql}
			inputBytes, _ := json.Marshal(input)
			_, err := sqliteTool.Execute(context.Background(), inputBytes)
			if err != nil {
				t.Errorf("expected %s to pass safety check, got error: %v", tc.name, err)
			}
		})
	}
}

func BenchmarkCSVTool_Read(b *testing.B) {
	csvTool := NewCSVTool()

	tmpDir := b.TempDir()
	csvPath := filepath.Join(tmpDir, "bench.csv")

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)
	writer.Write([]string{"col1", "col2", "col3"})
	for i := 0; i < 10000; i++ {
		writer.Write([]string{fmt.Sprintf("val%d_1", i), fmt.Sprintf("val%d_2", i), fmt.Sprintf("val%d_3", i)})
	}
	writer.Flush()
	os.WriteFile(csvPath, buf.Bytes(), 0644)

	input := map[string]any{"action": "read", "file_path": csvPath}
	inputBytes, _ := json.Marshal(input)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := csvTool.Execute(context.Background(), inputBytes)
		if err != nil {
			b.Fatal(err)
		}
	}
}
