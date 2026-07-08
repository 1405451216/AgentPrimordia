// workflow_evaluator.go — 条件评估、值比较、类型转换、模板渲染
//   - evaluateNodeCondition：节点执行条件
//   - evaluateTransitionCondition：转换条件（含 probability 概率分支）
//   - compareValues / toFloat64：通用比较与类型转换
//   - renderTemplate：占位符模板（支持 {{key}}）
//   - buildPrompt：构造 LLM 提示词
//   - applyInputMapping / applyOutputMapping：键重映射
package workflow

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
)

// evaluateNodeCondition 评估节点执行条件
func (w *WorkflowExecution) evaluateNodeCondition(condition *NodeCondition, input map[string]any) bool {
	if condition.CustomFunc != nil {
		return condition.CustomFunc(input)
	}

	fieldVal, exists := input[condition.Field]
	if !exists {
		return false
	}

	return compareValues(fieldVal, condition.Operator, condition.Value)
}

// evaluateTransitionCondition 评估转换条件
func (w *WorkflowExecution) evaluateTransitionCondition(transition *Transition, input map[string]any) bool {
	if transition.Condition == nil || transition.Condition.Type == "always" {
		return true
	}

	if transition.Condition.Type == "probability" {
		threshold := transition.Condition.Probability
		if threshold <= 0 {
			threshold = 0.5
		}
		if threshold > 1 {
			threshold = 1
		}
		return rand.Float64() < threshold
	}

	if transition.Condition.Field != "" {
		fieldVal, exists := input[transition.Condition.Field]
		if !exists {
			return false
		}
		return compareValues(fieldVal, transition.Condition.Operator, transition.Condition.Value)
	}

	return true
}

// compareValues 按 operator 比较两个值（支持 ==, !=, >, <, >=, <=）
func compareValues(left any, operator string, right any) bool {
	switch operator {
	case "==":
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right)
	case "!=":
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right)
	case ">":
		return toFloat64(left) > toFloat64(right)
	case "<":
		return toFloat64(left) < toFloat64(right)
	case ">=":
		return toFloat64(left) >= toFloat64(right)
	case "<=":
		return toFloat64(left) <= toFloat64(right)
	default:
		return true
	}
}

// toFloat64 尝试将任意类型转换为 float64（仅支持数值类型）
func toFloat64(v any) float64 {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case float64:
		return val
	case float32:
		return float64(val)
	default:
		return 0.0
	}
}

// buildPrompt 构造节点的 LLM 提示词：优先模板，其次 [name] + JSON 输入
func (w *WorkflowExecution) buildPrompt(node *WorkflowNode, input map[string]any) string {
	if node.Config != nil && node.Config.PromptTemplate != "" {
		return renderTemplate(node.Config.PromptTemplate, input)
	}

	promptParts := []string{fmt.Sprintf("[%s]", node.Name)}

	if input != nil {
		inputJSON, _ := json.MarshalIndent(input, "", "  ")
		promptParts = append(promptParts, fmt.Sprintf("\n输入数据:\n```json\n%s\n```", string(inputJSON)))
	}

	promptParts = append(promptParts, "\n请处理以上输入并返回结果。")

	return strings.Join(promptParts, "\n")
}

// applyInputMapping 按映射表重命名输入键，原键保留
func (w *WorkflowExecution) applyInputMapping(input map[string]any, mapping map[string]string) map[string]any {
	if len(mapping) == 0 {
		return input
	}

	result := make(map[string]any)
	for key, value := range input {
		result[key] = value
	}

	for newKey, sourceKey := range mapping {
		if val, exists := input[sourceKey]; exists {
			result[newKey] = val
		}
	}

	return result
}

// applyOutputMapping 按映射表重命名输出键，原键保留
func (w *WorkflowExecution) applyOutputMapping(output map[string]any, mapping map[string]string) map[string]any {
	if len(mapping) == 0 || output == nil {
		return output
	}

	result := make(map[string]any)
	for key, value := range output {
		result[key] = value
	}

	for newKey, sourceKey := range mapping {
		if val, exists := output[sourceKey]; exists {
			result[newKey] = val
		}
	}

	return result
}

// renderTemplate 简单占位符模板：支持 {{key}}
func renderTemplate(template string, data map[string]any) string {
	result := template
	for key, value := range data {
		placeholder := fmt.Sprintf("{{%s}}", key)
		valueStr := fmt.Sprintf("%v", value)
		result = strings.ReplaceAll(result, placeholder, valueStr)
	}
	return result
}
