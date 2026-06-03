package llm

import (
	"encoding/json"
	"testing"
)

func TestSchemaFromStruct_BasicTypes(t *testing.T) {
	type Basic struct {
		Name   string  `json:"name"`
		Age    int     `json:"age"`
		Score  float64 `json:"score"`
		Active bool    `json:"active"`
	}

	schema := SchemaFromStruct(Basic{})

	if schema.Name != "Basic" {
		t.Errorf("Name = %q, want %q", schema.Name, "Basic")
	}

	props, ok := schema.Schema["properties"].(map[string]any)
	if !ok {
		t.Fatal("properties should be map[string]any")
	}

	nameProp, ok := props["name"].(map[string]any)
	if !ok {
		t.Fatal("name property should be map[string]any")
	}
	if nameProp["type"] != "string" {
		t.Errorf("name type = %v, want string", nameProp["type"])
	}

	ageProp := props["age"].(map[string]any)
	if ageProp["type"] != "integer" {
		t.Errorf("age type = %v, want integer", ageProp["type"])
	}

	scoreProp := props["score"].(map[string]any)
	if scoreProp["type"] != "number" {
		t.Errorf("score type = %v, want number", scoreProp["type"])
	}

	activeProp := props["active"].(map[string]any)
	if activeProp["type"] != "boolean" {
		t.Errorf("active type = %v, want boolean", activeProp["type"])
	}
}

func TestSchemaFromStruct_RequiredFields(t *testing.T) {
	type WithOptional struct {
		Required string `json:"required"`
		Optional string `json:"optional,omitempty"`
	}

	schema := SchemaFromStruct(WithOptional{})

	required, ok := schema.Schema["required"].([]string)
	if !ok {
		t.Fatal("required should be []string")
	}

	found := false
	for _, r := range required {
		if r == "required" {
			found = true
		}
	}
	if !found {
		t.Error(`"required" should be in required list`)
	}

	for _, r := range required {
		if r == "optional" {
			t.Error(`"optional" with omitempty should NOT be in required list`)
		}
	}
}

func TestSchemaFromStruct_SliceType(t *testing.T) {
	type WithSlice struct {
		Tags []string `json:"tags"`
	}

	schema := SchemaFromStruct(WithSlice{})

	props := schema.Schema["properties"].(map[string]any)
	tagsProp := props["tags"].(map[string]any)

	if tagsProp["type"] != "array" {
		t.Errorf("tags type = %v, want array", tagsProp["type"])
	}

	items, ok := tagsProp["items"].(map[string]any)
	if !ok {
		t.Fatal("tags items should be map[string]any")
	}
	if items["type"] != "string" {
		t.Errorf("tags items type = %v, want string", items["type"])
	}
}

func TestSchemaFromStruct_NestedStruct(t *testing.T) {
	type Address struct {
		City   string `json:"city"`
		Street string `json:"street"`
	}
	type Person struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}

	schema := SchemaFromStruct(Person{})

	props := schema.Schema["properties"].(map[string]any)
	addrProp := props["address"].(map[string]any)

	if addrProp["type"] != "object" {
		t.Errorf("address type = %v, want object", addrProp["type"])
	}

	addrProps, ok := addrProp["properties"].(map[string]any)
	if !ok {
		t.Fatal("address properties should be map[string]any")
	}

	cityProp := addrProps["city"].(map[string]any)
	if cityProp["type"] != "string" {
		t.Errorf("city type = %v, want string", cityProp["type"])
	}
}

func TestSchemaFromStruct_JsonschemaTag(t *testing.T) {
	type WithDesc struct {
		Status string `json:"status" jsonschema:"description=Current status,enum=active,enum=inactive,enum=pending"`
		Count  int    `json:"count" jsonschema:"description=Item count,minimum=0,maximum=100"`
	}

	schema := SchemaFromStruct(WithDesc{})

	props := schema.Schema["properties"].(map[string]any)

	statusProp := props["status"].(map[string]any)
	if statusProp["description"] != "Current status" {
		t.Errorf("status description = %v, want 'Current status'", statusProp["description"])
	}
	enums, ok := statusProp["enum"].([]string)
	if !ok || len(enums) != 3 {
		t.Fatalf("status enum = %v, want 3 items", statusProp["enum"])
	}
	if enums[0] != "active" || enums[1] != "inactive" || enums[2] != "pending" {
		t.Errorf("status enum values = %v", enums)
	}

	countProp := props["count"].(map[string]any)
	if countProp["description"] != "Item count" {
		t.Errorf("count description = %v", countProp["description"])
	}
	if countProp["minimum"] != 0 {
		t.Errorf("count minimum = %v, want 0", countProp["minimum"])
	}
	if countProp["maximum"] != 100 {
		t.Errorf("count maximum = %v, want 100", countProp["maximum"])
	}
}

func TestSchemaFromStruct_PointerField(t *testing.T) {
	type WithPointer struct {
		Label *string `json:"label,omitempty"`
	}

	schema := SchemaFromStruct(WithPointer{})

	props := schema.Schema["properties"].(map[string]any)
	labelProp := props["label"].(map[string]any)

	if labelProp["type"] != "string" {
		t.Errorf("label type = %v, want string (dereferenced)", labelProp["type"])
	}
}

func TestSchemaFromStruct_SliceOfInt(t *testing.T) {
	type WithIntSlice struct {
		Numbers []int `json:"numbers"`
	}

	schema := SchemaFromStruct(WithIntSlice{})

	props := schema.Schema["properties"].(map[string]any)
	numProp := props["numbers"].(map[string]any)
	items := numProp["items"].(map[string]any)

	if items["type"] != "integer" {
		t.Errorf("numbers items type = %v, want integer", items["type"])
	}
}

func TestSchemaFromStruct_NestedSlice(t *testing.T) {
	type Matrix struct {
		Rows [][]float64 `json:"rows"`
	}

	schema := SchemaFromStruct(Matrix{})

	props := schema.Schema["properties"].(map[string]any)
	rowsProp := props["rows"].(map[string]any)
	outerItems := rowsProp["items"].(map[string]any)

	if outerItems["type"] != "array" {
		t.Errorf("rows items type = %v, want array", outerItems["type"])
	}

	innerItems := outerItems["items"].(map[string]any)
	if innerItems["type"] != "number" {
		t.Errorf("inner items type = %v, want number", innerItems["type"])
	}
}

func TestSchemaFromStruct_CustomName(t *testing.T) {
	type MyData struct {
		Value string `json:"value"`
	}

	schema := SchemaFromStruct(MyData{}, WithSchemaName("CustomName"))

	if schema.Name != "CustomName" {
		t.Errorf("Name = %q, want %q", schema.Name, "CustomName")
	}
}

func TestSchemaFromStruct_Strict(t *testing.T) {
	type StrictData struct {
		X int `json:"x"`
	}

	schema := SchemaFromStruct(StrictData{}, WithStrictMode())

	if !schema.Strict {
		t.Error("Strict should be true")
	}
}

func TestSchemaFromStruct_EmbeddedStruct(t *testing.T) {
	type Base struct {
		ID string `json:"id"`
	}
	type Extended struct {
		Base
		Name string `json:"name"`
	}

	schema := SchemaFromStruct(Extended{})

	props := schema.Schema["properties"].(map[string]any)

	if _, ok := props["id"]; !ok {
		t.Error("embedded 'id' field should be in properties")
	}
	if _, ok := props["name"]; !ok {
		t.Error("'name' field should be in properties")
	}
}

func TestSchemaFromStruct_SkipUnexported(t *testing.T) {
	type WithUnexported struct {
		Public string `json:"public"`
		_      string
	}

	schema := SchemaFromStruct(WithUnexported{})

	props := schema.Schema["properties"].(map[string]any)
	if _, ok := props["public"]; !ok {
		t.Error("public field should be in properties")
	}
	if _, ok := props["private"]; ok {
		t.Error("private field should NOT be in properties")
	}
}

func TestSchemaFromStruct_MapOutput(t *testing.T) {
	type Simple struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	schema := SchemaFromStruct(Simple{})

	raw, err := json.Marshal(schema.Schema)
	if err != nil {
		t.Fatalf("Marshal schema: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if m["type"] != "object" {
		t.Errorf("type = %v, want object", m["type"])
	}
}
