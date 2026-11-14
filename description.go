package playground

import (
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	contract "github.com/NARUBROWN/spine/contract/validator"
)

var _ contract.SchemaDescriber = (*Validator)(nil)
var enumTokens = regexp.MustCompile(`'[^']*'|[^\s]+`)

// DescribeSchema는 완성된 검증기의 설정만 읽고 사용자 규칙이나 Validate를 실행하지 않습니다.
// 표현할 수 없는 규칙은 진단으로 남기며 내장 이름을 덮어쓴 사용자 규칙도 추론하지 않습니다.
func (v *Validator) DescribeSchema(t reflect.Type) (contract.SchemaDescription, error) {
	result := contract.SchemaDescription{Fields: []contract.FieldSchema{}, Unresolved: []string{}}
	if v == nil || v.engine == nil {
		return result, fmt.Errorf("%w: validator is not initialized", contract.ErrInvalidValidator)
	}
	if t == nil {
		return result, fmt.Errorf("%w: type is nil", contract.ErrInvalidValue)
	}
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return result, fmt.Errorf("%w: expected a struct type", contract.ErrInvalidValue)
	}
	visited := map[reflect.Type]bool{}
	var walk func(reflect.Type, []int)
	walk = func(current reflect.Type, prefix []int) {
		if visited[current] {
			return
		}
		visited[current] = true
		defer delete(visited, current)
		if v.structRules[current] {
			result.Unresolved = append(result.Unresolved, current.String()+": custom struct validation")
		}
		for i := 0; i < current.NumField(); i++ {
			f := current.Field(i)
			base := f.Type
			if base.Kind() == reflect.Pointer {
				base = base.Elem()
			}
			if !f.IsExported() && !(f.Anonymous && base.Kind() == reflect.Struct) {
				continue
			}
			index := append(append([]int(nil), prefix...), i)
			tag := f.Tag.Get(v.validationTag)
			if f.Type.Kind() == reflect.Struct && !f.Anonymous && tag == "" {
				result.Unresolved = append(result.Unresolved, current.String()+"."+f.Name+": implicit nested validation presence")
			}
			fieldTag, conditional, issues := v.conditionalRules(current, f, prefix, tag)
			result.ConditionalRequired = append(result.ConditionalRequired, conditional...)
			constraint, required, skip, unresolved := v.fieldSchema(f.Type, fieldTag)
			unresolved = append(unresolved, issues...)
			for _, rule := range unresolved {
				result.Unresolved = append(result.Unresolved, current.String()+"."+f.Name+": "+rule)
			}
			if tag != "" || skip {
				encoded, _ := json.Marshal(constraint)
				result.Fields = append(result.Fields, contract.FieldSchema{Index: index, Required: required, SkipNested: skip, Schema: encoded})
			}
			if base.Kind() == reflect.Struct && !skip {
				walk(base, index)
			}
		}
	}
	walk(t, nil)
	return result, nil
}

func (v *Validator) fieldSchema(t reflect.Type, tag string) (map[string]any, bool, bool, []string) {
	base := t
	if base.Kind() == reflect.Pointer {
		base = base.Elem()
	}
	container := base.Kind() == reflect.Slice || base.Kind() == reflect.Array || base.Kind() == reflect.Map
	if tag == "-" {
		return map[string]any{}, false, true, nil
	}
	if tag == "" {
		return map[string]any{}, false, container, nil
	}
	parts := strings.Split(tag, ",")
	optional := ""
	if parts[0] == "omitempty" || parts[0] == "omitnil" {
		optional = parts[0]
		parts = parts[1:]
	}
	clauses := []any{}
	unknown := []string{}
	zeroAllowed := true
	dive := false
	for i, part := range parts {
		name, param, hasParam := strings.Cut(part, "=")
		param = strings.ReplaceAll(strings.ReplaceAll(param, "0x2C", ","), "0x7C", "|")
		if name == "dive" {
			dive = true
			if !container {
				unknown = append(unknown, "dive on a non-container")
				break
			}
			element := base.Elem()
			if element.Kind() == reflect.Pointer {
				element = element.Elem()
			}
			if element.Kind() == reflect.Struct {
				unknown = append(unknown, "dive over struct elements: nested field and struct rules require a separate type description")
			}
			if i+1 < len(parts) {
				if strings.Contains(strings.Join(parts[i+1:], ","), "keys") {
					unknown = append(unknown, "map key validation")
					dive = false
					break
				}
				child, childRequired, _, issues := v.fieldSchema(base.Elem(), strings.Join(parts[i+1:], ","))
				unknown = append(unknown, issues...)
				if base.Kind() == reflect.Array {
					prefix := make([]any, base.Len())
					for i := range prefix {
						prefix[i] = child
					}
					clause := map[string]any{}
					if len(prefix) > 0 {
						clause["prefixItems"] = prefix
					}
					if childRequired && base.Len() > 0 {
						clause["minItems"] = base.Len()
						zeroAllowed = false
					}
					clauses = append(clauses, clause)
					break
				}
				keyword := "items"
				if base.Kind() == reflect.Map {
					keyword = "additionalProperties"
				}
				clauses = append(clauses, map[string]any{keyword: child})
			}
			break
		}
		if v.customRules[name] {
			unknown = append(unknown, name+": custom rule")
			continue
		}
		if name == "omitempty" || name == "omitnil" || strings.Contains(name, "|") {
			unknown = append(unknown, part)
			continue
		}
		var clause map[string]any
		passZero := false
		switch name {
		case "required":
			if hasParam {
				unknown = append(unknown, part)
				continue
			}
			if base.Kind() == reflect.Array {
				unknown = append(unknown, "required on an array")
				clause = map[string]any{}
			} else if t.Kind() == reflect.Pointer || container {
				clause = map[string]any{"not": map[string]any{"type": "null"}}
			} else {
				switch base.Kind() {
				case reflect.String:
					clause = map[string]any{"minLength": 1}
				case reflect.Bool:
					clause = map[string]any{"const": true}
				case reflect.Struct:
					unknown = append(unknown, "required on a struct")
					clause = map[string]any{}
				default:
					clause = map[string]any{"not": map[string]any{"const": 0}}
				}
			}
		case "email", "uuid", "uuid4", "url", "uri":
			if base.Kind() != reflect.String || hasParam {
				unknown = append(unknown, part)
				continue
			}
			format := name
			if name == "uuid4" {
				format = "uuid"
			}
			if name == "url" {
				format = "uri"
			}
			clause = map[string]any{"format": format, "minLength": 1}
		case "min", "max", "gte", "lte", "gt", "lt", "len":
			if !hasParam {
				unknown = append(unknown, part)
				continue
			}
			if base.Kind() == reflect.String || container {
				bound, err := strconv.ParseUint(param, 10, 63)
				if err != nil {
					unknown = append(unknown, part)
					continue
				}
				if base.Kind() == reflect.Array {
					length := uint64(base.Len())
					pass := false
					switch name {
					case "min", "gte":
						pass = length >= bound
					case "max", "lte":
						pass = length <= bound
					case "len":
						pass = length == bound
					case "gt":
						pass = length > bound
					case "lt":
						pass = length < bound
					}
					clause, passZero = map[string]any{}, pass
					if !pass {
						clause["not"] = map[string]any{}
					}
					break
				}
				minimum, maximum := "minLength", "maxLength"
				if base.Kind() == reflect.Map {
					minimum, maximum = "minProperties", "maxProperties"
				} else if container {
					minimum, maximum = "minItems", "maxItems"
				}
				clause = map[string]any{}
				switch name {
				case "min", "gte":
					clause[minimum] = bound
					passZero = bound == 0
				case "max", "lte":
					clause[maximum] = bound
					passZero = true
				case "len":
					clause[minimum], clause[maximum] = bound, bound
					passZero = bound == 0
				case "gt":
					clause[minimum] = bound + 1
				case "lt":
					if bound == 0 {
						clause["not"] = map[string]any{}
					} else {
						clause[maximum] = bound - 1
						passZero = true
					}
				}
			} else if numericKind(base.Kind()) {
				value, err := strconv.ParseFloat(param, 64)
				if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
					unknown = append(unknown, part)
					continue
				}
				if base.Kind() != reflect.Float32 && base.Kind() != reflect.Float64 {
					if _, err := strconv.ParseInt(param, 10, 64); err != nil {
						unknown = append(unknown, part)
						continue
					}
				}
				canonical := strconv.FormatFloat(value, 'g', -1, 64)
				if base.Kind() != reflect.Float32 && base.Kind() != reflect.Float64 {
					integer, _ := strconv.ParseInt(param, 10, 64)
					canonical = strconv.FormatInt(integer, 10)
				}
				number := json.Number(canonical)
				clause = map[string]any{}
				switch name {
				case "min", "gte":
					clause["minimum"] = number
					passZero = value <= 0
				case "max", "lte":
					clause["maximum"] = number
					passZero = value >= 0
				case "gt":
					clause["exclusiveMinimum"] = number
					passZero = value < 0
				case "lt":
					clause["exclusiveMaximum"] = number
					passZero = value > 0
				case "len":
					clause["const"] = number
					passZero = value == 0
				}
			} else {
				unknown = append(unknown, part)
				continue
			}
		case "oneof":
			values := []any{}
			tokens := enumTokens.FindAllString(param, -1)
			if !hasParam || len(tokens) == 0 {
				unknown = append(unknown, part)
				continue
			}
			valid := true
			for _, token := range tokens {
				token = strings.Trim(token, "'")
				if base.Kind() == reflect.String {
					values = append(values, token)
					passZero = passZero || token == ""
				} else if numericKind(base.Kind()) && base.Kind() != reflect.Float32 && base.Kind() != reflect.Float64 {
					value, err := strconv.ParseInt(token, 10, 64)
					if err != nil {
						valid = false
						break
					}
					values = append(values, json.Number(strconv.FormatInt(value, 10)))
					passZero = passZero || value == 0
				} else {
					valid = false
					break
				}
			}
			if !valid {
				unknown = append(unknown, part)
				continue
			}
			clause = map[string]any{"enum": values}
		default:
			unknown = append(unknown, part)
			continue
		}
		if t.Kind() == reflect.Pointer {
			clause = map[string]any{"allOf": []any{map[string]any{"not": map[string]any{"type": "null"}}, clause}}
			passZero = false
		}
		clauses = append(clauses, clause)
		zeroAllowed = zeroAllowed && passZero
	}
	result := map[string]any{}
	if len(clauses) > 0 {
		result["allOf"] = clauses
	}
	if optional != "" {
		var zero any
		if optional == "omitnil" || t.Kind() == reflect.Pointer || base.Kind() == reflect.Slice || base.Kind() == reflect.Map {
			zero = nil
		} else {
			switch base.Kind() {
			case reflect.String:
				zero = ""
			case reflect.Bool:
				zero = false
			default:
				if numericKind(base.Kind()) {
					zero = 0
				} else {
					unknown = append(unknown, optional+" on a composite value")
					return map[string]any{}, false, true, unknown
				}
			}
		}
		if optional == "omitnil" && t.Kind() != reflect.Pointer && base.Kind() != reflect.Slice && base.Kind() != reflect.Map {
			unknown = append(unknown, "omitnil on a non-nilable value")
			return result, !zeroAllowed, container && !dive, unknown
		}
		result = map[string]any{"anyOf": []any{map[string]any{"const": zero}, result}}
		zeroAllowed = true
	}
	return result, !zeroAllowed, container && !dive, unknown
}

func numericKind(k reflect.Kind) bool {
	return k >= reflect.Int && k <= reflect.Float64
}
