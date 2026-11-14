package playground

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"

	contract "github.com/NARUBROWN/spine/contract/validator"
)

func (v *Validator) conditionalRules(parent reflect.Type, field reflect.StructField, prefix []int, tag string) (string, []contract.ConditionalRequired, []string) {
	if !strings.Contains(tag, "required_if=") || v.customRules["required_if"] {
		return tag, nil, nil
	}
	var rules []contract.ConditionalRequired
	var unresolved []string
	parts := strings.Split(tag, ",")
	remaining := make([]string, 0, len(parts))
	optional := false
	for i, part := range parts {
		name, parameter, hasParameter := strings.Cut(part, "=")
		if name == "dive" {
			remaining = append(remaining, parts[i:]...)
			break
		}
		if name == "omitempty" || name == "omitnil" || name == "omitzero" {
			optional = true
		}
		if name != "required_if" || !hasParameter || strings.Contains(part, "|") {
			remaining = append(remaining, part)
			continue
		}
		rule, ok := conditionalRule(parent, field, prefix, parameter)
		if optional || !ok {
			unresolved = append(unresolved, part)
			continue
		}
		rules = append(rules, rule)
	}
	return strings.Join(remaining, ","), rules, unresolved
}

func conditionalRule(parent reflect.Type, field reflect.StructField, prefix []int, parameter string) (contract.ConditionalRequired, bool) {
	parameter = strings.NewReplacer("0x2C", ",", "0x7C", "|").Replace(parameter)
	tokens := enumTokens.FindAllString(parameter, -1)
	if len(tokens) == 0 || len(tokens)%2 != 0 || field.Type == reflect.TypeFor[json.Number]() {
		return contract.ConditionalRequired{}, false
	}
	rule := contract.ConditionalRequired{Index: fieldIndex(prefix, field.Index)}
	constraint := ""
	switch field.Type.Kind() {
	case reflect.String:
		constraint = `{"minLength":1}`
	case reflect.Pointer, reflect.Slice, reflect.Map:
		constraint = `{"not":{"type":"null"}}`
	case reflect.Bool:
		constraint = `{"const":true}`
	default:
		if !numericKind(field.Type.Kind()) {
			return contract.ConditionalRequired{}, false
		}
		constraint = `{"not":{"const":0}}`
	}
	rule.Schema = json.RawMessage(constraint)
	seen := map[string]bool{}
	for i := 0; i < len(tokens); i += 2 {
		name := strings.ReplaceAll(tokens[i], "'", "")
		value := strings.ReplaceAll(tokens[i+1], "'", "")
		ref, ok := parent.FieldByName(name)
		if !ok || !ref.IsExported() || len(ref.Index) != 1 || seen[name] {
			return contract.ConditionalRequired{}, false
		}
		seen[name] = true
		encoded := conditionalValue(ref.Type, value)
		if encoded == nil {
			return contract.ConditionalRequired{}, false
		}
		rule.When = append(rule.When, contract.FieldCondition{Index: fieldIndex(prefix, ref.Index), Value: encoded})
	}
	return rule, true
}

// 필드가 생략됐을 때도 성립하는 비교는 현재 명세 계약으로 표현하지 않습니다.
func conditionalValue(t reflect.Type, value string) json.RawMessage {
	if t == reflect.TypeFor[json.Number]() {
		return nil
	}
	switch t.Kind() {
	case reflect.String:
		if value != "" {
			encoded, _ := json.Marshal(value)
			return encoded
		}
	case reflect.Bool:
		if value == "true" {
			return json.RawMessage("true")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if n, err := strconv.ParseInt(value, 10, t.Bits()); err == nil && n != 0 {
			return json.RawMessage(strconv.FormatInt(n, 10))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if n, err := strconv.ParseUint(value, 10, t.Bits()); err == nil && n != 0 {
			return json.RawMessage(strconv.FormatUint(n, 10))
		}
	}
	return nil
}

func fieldIndex(prefix, index []int) []int {
	result := make([]int, 0, len(prefix)+len(index))
	result = append(result, prefix...)
	return append(result, index...)
}
