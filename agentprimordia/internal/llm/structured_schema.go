package llm

import (
	"reflect"
	"strconv"
	"strings"
)

// SchemaOption 配置 SchemaFromStruct 的行为
type SchemaOption func(*schemaConfig)

type schemaConfig struct {
	name   string
	strict bool
}

// WithSchemaName 设置生成的 Schema 名称（默认使用类型名）
func WithSchemaName(name string) SchemaOption {
	return func(c *schemaConfig) { c.name = name }
}

// WithStrictMode 启用严格模式（additionalProperties: false）
func WithStrictMode() SchemaOption {
	return func(c *schemaConfig) { c.strict = true }
}

// SchemaFromStruct 从 Go struct 类型/实例生成 JSON Schema 定义
// 支持基本类型、切片、嵌套 struct、指针、json tag、jsonschema tag
func SchemaFromStruct(v any, opts ...SchemaOption) *SchemaDef {
	if v == nil {
		return &SchemaDef{Schema: map[string]any{"type": "null"}}
	}
	cfg := &schemaConfig{}
	for _, o := range opts {
		o(cfg)
	}

	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	name := cfg.name
	if name == "" {
		name = t.Name()
	}

	schema := structToSchema(t)
	result := &SchemaDef{
		Name:   name,
		Schema: schema,
		Strict: cfg.strict,
	}

	if cfg.strict {
		schema["additionalProperties"] = false
	}

	return result
}

// structToSchema 将 Go struct 类型转换为 JSON Schema map
func structToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	properties := make(map[string]any)
	var required []string

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		if !field.IsExported() {
			continue
		}

		if field.Anonymous {
			embedded := structToSchema(field.Type)
			if embeddedProps, ok := embedded["properties"].(map[string]any); ok {
				for k, v := range embeddedProps {
					properties[k] = v
				}
			}
			if embeddedReq, ok := embedded["required"].([]string); ok {
				required = append(required, embeddedReq...)
			}
			continue
		}

		jsonKey, omitempty := parseJSONTag(field.Tag.Get("json"))
		if jsonKey == "-" {
			continue
		}

		prop := typeToSchema(field.Type)

		applyJsonschemaTag(prop, field.Tag.Get("jsonschema"))

		properties[jsonKey] = prop

		if !omitempty {
			required = append(required, jsonKey)
		}
	}

	result := map[string]any{
		"type":       "object",
		"properties": properties,
	}

	if len(required) > 0 {
		result["required"] = required
	}

	return result
}

// typeToSchema 将 Go 类型转换为 JSON Schema 属性
func typeToSchema(t reflect.Type) map[string]any {
	if t.Kind() == reflect.Ptr {
		return typeToSchema(t.Elem())
	}

	switch t.Kind() {
	case reflect.String:
		return map[string]any{"type": "string"}

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}

	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}

	case reflect.Bool:
		return map[string]any{"type": "boolean"}

	case reflect.Slice:
		if t.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string", "format": "byte"}
		}
		items := typeToSchema(t.Elem())
		return map[string]any{
			"type":  "array",
			"items": items,
		}

	case reflect.Struct:
		return structToSchema(t)

	case reflect.Map:
		if t.Key().Kind() == reflect.String {
			return map[string]any{
				"type":                 "object",
				"additionalProperties": typeToSchema(t.Elem()),
			}
		}

	default:
		return map[string]any{"type": "string"}
	}

	return map[string]any{"type": "string"}
}

// parseJSONTag 解析 json tag，返回字段名和是否 omitempty
func parseJSONTag(tag string) (name string, omitempty bool) {
	if tag == "" {
		return "", false
	}

	parts := strings.Split(tag, ",")
	name = parts[0]
	if name == "" {
		name = ""
	}

	for _, p := range parts[1:] {
		if p == "omitempty" {
			omitempty = true
		}
	}

	return
}

// applyJsonschemaTag 解析 jsonschema tag 并应用到 Schema 属性
// 格式: "description=xxx,enum=a,enum=b,minimum=0,maximum=100"
func applyJsonschemaTag(prop map[string]any, tag string) {
	if tag == "" {
		return
	}

	parts := strings.Split(tag, ",")
	var enums []string

	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "description=") {
			prop["description"] = strings.TrimPrefix(p, "description=")
		} else if strings.HasPrefix(p, "enum=") {
			enums = append(enums, strings.TrimPrefix(p, "enum="))
		} else if strings.HasPrefix(p, "minimum=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(p, "minimum=")); err == nil {
				prop["minimum"] = v
			}
		} else if strings.HasPrefix(p, "maximum=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(p, "maximum=")); err == nil {
				prop["maximum"] = v
			}
		} else if strings.HasPrefix(p, "minLength=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(p, "minLength=")); err == nil {
				prop["minLength"] = v
			}
		} else if strings.HasPrefix(p, "maxLength=") {
			if v, err := strconv.Atoi(strings.TrimPrefix(p, "maxLength=")); err == nil {
				prop["maxLength"] = v
			}
		} else if strings.HasPrefix(p, "pattern=") {
			prop["pattern"] = strings.TrimPrefix(p, "pattern=")
		} else if strings.HasPrefix(p, "format=") {
			prop["format"] = strings.TrimPrefix(p, "format=")
		} else if p == "required" {
			prop["required"] = true
		}
	}

	if len(enums) > 0 {
		prop["enum"] = enums
	}
}
