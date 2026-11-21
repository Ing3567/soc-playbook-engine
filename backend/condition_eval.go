package main

import (
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
)

// EvaluateEdgeCondition evaluates a structured decision condition against the previous node output
// and full execution context (alert, prev, nodes).
func EvaluateEdgeCondition(cond *EdgeCondition, prevOutput map[string]any, fullCtx map[string]any) bool {
	if cond == nil {
		return false
	}
	field := strings.TrimSpace(cond.Field)
	if field == "" {
		return false
	}
	op := strings.ToLower(strings.TrimSpace(cond.Op))
	actual, found := lookupConditionField(field, prevOutput, fullCtx)

	switch op {
	case "is_true":
		return found && isTruthyValue(actual)
	case "is_false":
		return !found || !isTruthyValue(actual)
	case "==", "eq":
		if !found {
			return false
		}
		return compareEqual(actual, cond.Value)
	case "!=", "neq":
		if !found {
			return cond.Value != nil
		}
		return !compareEqual(actual, cond.Value)
	case ">", "gt":
		a, b, ok := toNumericPair(actual, cond.Value)
		return found && ok && a > b
	case ">=", "gte":
		a, b, ok := toNumericPair(actual, cond.Value)
		return found && ok && a >= b
	case "<", "lt":
		a, b, ok := toNumericPair(actual, cond.Value)
		return found && ok && a < b
	case "<=", "lte":
		a, b, ok := toNumericPair(actual, cond.Value)
		return found && ok && a <= b
	case "contains":
		if !found || actual == nil {
			return false
		}
		return valueContains(actual, cond.Value)
	default:
		return false
	}
}

// PickDecisionEdge selects the outgoing branch from a decision node.
// Structured Edge.Condition rules take priority; if no edge defines a condition,
// it falls back to the legacy SOC verdict heuristic for backwards compatibility.
func PickDecisionEdge(outEdges []Edge, prevOutput map[string]any, fullCtx map[string]any) *Edge {
	if len(outEdges) == 0 {
		return nil
	}
	if len(outEdges) == 1 {
		e := outEdges[0]
		return &e
	}

	hasStructured := false
	for _, e := range outEdges {
		if e.Condition != nil && strings.TrimSpace(e.Condition.Field) != "" {
			hasStructured = true
			break
		}
	}

	if hasStructured {
		var defaultEdge *Edge
		for i := range outEdges {
			e := outEdges[i]
			if e.Condition != nil && strings.TrimSpace(e.Condition.Field) != "" {
				if EvaluateEdgeCondition(e.Condition, prevOutput, fullCtx) {
					return &e
				}
			} else if defaultEdge == nil {
				copyEdge := e
				defaultEdge = &copyEdge
			}
		}
		if defaultEdge != nil {
			return defaultEdge
		}
		last := outEdges[len(outEdges)-1]
		return &last
	}

	// Legacy heuristic fallback when SOP edges have no structured condition.
	isPositive := isLegacyPositiveOutput(prevOutput)
	if isPositive {
		for i := range outEdges {
			lbl := strings.TrimSpace(outEdges[i].Label)
			if lbl == "ใช่" || strings.EqualFold(lbl, "yes") {
				e := outEdges[i]
				return &e
			}
		}
		e := outEdges[0]
		return &e
	}

	for i := range outEdges {
		lbl := strings.TrimSpace(outEdges[i].Label)
		if lbl == "ไม่ใช่" || strings.EqualFold(lbl, "no") {
			e := outEdges[i]
			return &e
		}
	}
	last := outEdges[len(outEdges)-1]
	return &last
}

func isLegacyPositiveOutput(out map[string]any) bool {
	if out == nil {
		return false
	}
	if n, ok := toFloat64(out["malicious_count"]); ok && n > 0 {
		return true
	}
	if n, ok := toFloat64(out["confidence_score"]); ok && n >= 50 {
		return true
	}
	if n, ok := toFloat64(out["risk_score"]); ok && n >= 50 {
		return true
	}
	if b, ok := out["is_malicious"].(bool); ok && b {
		return true
	}
	if b, ok := out["alert"].(bool); ok && b {
		return true
	}
	if b, ok := out["matched"].(bool); ok && b {
		return true
	}
	if mc, ok := out["matched_cases"]; ok && mc != nil {
		rv := reflect.ValueOf(mc)
		if (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Len() > 0 {
			return true
		}
	}
	return false
}

func lookupConditionField(path string, prevOutput map[string]any, fullCtx map[string]any) (any, bool) {
	ctx := make(map[string]any, 4)
	for k, v := range fullCtx {
		ctx[k] = v
	}
	if prevOutput != nil {
		ctx["prev"] = prevOutput
	}

	// Shorthand without dot prefix checks prev output first.
	if !strings.Contains(path, ".") && prevOutput != nil {
		if v, ok := prevOutput[path]; ok {
			return v, true
		}
	}

	parts := strings.Split(path, ".")
	var current any = ctx
	for _, part := range parts {
		m, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		next, exists := m[part]
		if !exists {
			return nil, false
		}
		current = next
	}
	return current, true
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n)
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case int32:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint64:
		return float64(n), true
	case string:
		trimmed := strings.TrimSpace(n)
		if trimmed == "" {
			return 0, false
		}
		f, err := strconv.ParseFloat(trimmed, 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func toNumericPair(a, b any) (float64, float64, bool) {
	fa, okA := toFloat64(a)
	fb, okB := toFloat64(b)
	return fa, fb, okA && okB
}

func compareEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if ba, okA := a.(bool); okA {
		if bb, okB := b.(bool); okB {
			return ba == bb
		}
		if sb, okB := b.(string); okB {
			return strings.EqualFold(strconv.FormatBool(ba), strings.TrimSpace(sb))
		}
	}
	if fa, fb, ok := toNumericPair(a, b); ok {
		return fa == fb
	}
	return strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", a)), strings.TrimSpace(fmt.Sprintf("%v", b)))
}

func isTruthyValue(v any) bool {
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s != "" && s != "false" && s != "0" && s != "no"
	default:
		if f, ok := toFloat64(v); ok {
			return f != 0
		}
		rv := reflect.ValueOf(v)
		switch rv.Kind() {
		case reflect.Slice, reflect.Array, reflect.Map:
			return rv.Len() > 0
		}
		return true
	}
}

func valueContains(actual, target any) bool {
	if target == nil {
		return false
	}
	needle := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", target)))
	if needle == "" {
		return false
	}

	rv := reflect.ValueOf(actual)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		for i := 0; i < rv.Len(); i++ {
			elem := strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", rv.Index(i).Interface())))
			if elem == needle || strings.Contains(elem, needle) {
				return true
			}
		}
		return false
	}

	haystack := strings.ToLower(fmt.Sprintf("%v", actual))
	return strings.Contains(haystack, needle)
}
