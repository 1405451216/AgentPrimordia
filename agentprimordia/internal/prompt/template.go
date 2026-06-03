package prompt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"text/template"
)

// Template 是 Prompt 模板引擎，支持变量注入、条件渲染和循环
type Template struct {
	template   string
	variables  map[string]any
	validators []ValidatorFunc
	delimiters [2]string // 左右分隔符，默认 {{ 和 }}
}

// ValidatorFunc 是模板验证函数类型
type ValidatorFunc func(variables map[string]any) error

// NewTemplate 创建新的模板实例
func NewTemplate(tmpl string) *Template {
	return &Template{
		template:   tmpl,
		variables:  make(map[string]any),
		delimiters: [2]string{"{{", "}}"},
	}
}

// WithDelimiters 设置自定义分隔符
func (t *Template) WithDelimiters(left, right string) *Template {
	t.delimiters[0] = left
	t.delimiters[1] = right
	return t
}

// WithVar 注入单个变量（支持链式调用）
func (t *Template) WithVar(key string, value any) *Template {
	t.variables[key] = value
	return t
}

// WithVars 批量注入变量
func (t *Template) WithVars(vars map[string]any) *Template {
	for k, v := range vars {
		t.variables[k] = v
	}
	return t
}

// AddValidator 添加验证函数
func (t *Template) AddValidator(fn ValidatorFunc) *Template {
	t.validators = append(t.validators, fn)
	return t
}

// Render 渲染模板，返回最终的 Prompt 文本
func (t *Template) Render() (string, error) {
	// 执行验证
	for _, validator := range t.validators {
		if err := validator(t.variables); err != nil {
			return "", fmt.Errorf("template validation failed: %w", err)
		}
	}

	// 使用 Go 标准库 text/template 渲染
	tmpl, err := template.New("prompt").Delims(t.delimiters[0], t.delimiters[1]).Funcs(
		template.FuncMap{
			"json":      toJSON,
			"indent":    indentJSON,
			"upper":     strings.ToUpper,
			"lower":     strings.ToLower,
			"title":     titleString,
			"join":      strings.Join,
			"contains":  strings.Contains,
			"hasPrefix": strings.HasPrefix,
			"hasSuffix": strings.HasSuffix,
			"replace":   strings.ReplaceAll,
			"trimSpace": strings.TrimSpace,
			"default":   defaultVal,
			"coalesce":  coalesce,
		},
	).Parse(t.template)
	if err != nil {
		return "", fmt.Errorf("parse template error: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, t.variables); err != nil {
		return "", fmt.Errorf("execute template error: %w", err)
	}

	return buf.String(), nil
}

// MustRender 类似 Render，但出错时 panic
func (t *Template) MustRender() string {
	result, err := t.Render()
	if err != nil {
		panic(fmt.Sprintf("prompt.MustRender() error: %v", err))
	}
	return result
}

// GetVariables 获取当前所有变量
func (t *Template) GetVariables() map[string]any {
	return t.variables
}

// GetRawTemplate 获取原始模板文本
func (t *Template) GetRawTemplate() string {
	return t.template
}

// Clone 克隆模板实例（保留模板文本，清空变量）
func (t *Template) Clone() *Template {
	return &Template{
		template:   t.template,
		variables:  make(map[string]any),
		delimiters: t.delimiters,
	}
}

// ===== 辅助函数 =====

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func indentJSON(spaces int, v any) string {
	b, _ := json.MarshalIndent(v, "", strings.Repeat(" ", spaces))
	return string(b)
}

func defaultVal(defaultValue any, v any) any {
	if v == nil || reflect.ValueOf(v).IsZero() {
		return defaultValue
	}
	return v
}

func coalesce(values ...any) any {
	for _, v := range values {
		if v != nil && !reflect.ValueOf(v).IsZero() {
			return v
		}
	}
	return nil
}

func titleString(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ===== 预定义验证器 =====

// RequireVars 要求必须包含指定变量
func RequireVars(required ...string) ValidatorFunc {
	return func(vars map[string]any) error {
		var missing []string
		for _, key := range required {
			if _, ok := vars[key]; !ok {
				missing = append(missing, key)
			}
		}
		if len(missing) > 0 {
			return fmt.Errorf("missing required variables: %v", missing)
		}
		return nil
	}
}

// MaxLength 限制最终输出长度
func MaxLength(max int) ValidatorFunc {
	return func(vars map[string]any) error {
		// 这个验证器需要在 Render 后检查，这里仅作为示例
		return nil
	}
}

// NoEmptyStrings 禁止空字符串值
func NoEmptyStrings(keys ...string) ValidatorFunc {
	return func(vars map[string]any) error {
		for _, key := range keys {
			if val, ok := vars[key].(string); ok && val == "" {
				return fmt.Errorf("variable '%s' cannot be empty", key)
			}
		}
		return nil
	}
}
