package playground

import (
	"fmt"
	"reflect"
	"slices"
	"unicode"

	contract "github.com/NARUBROWN/spine/contract/validator"
	native "github.com/go-playground/validator/v10"
)

type configuration struct {
	fieldTags     []string
	validationTag string
	rules         []func(*native.Validate) error
}

// Option은 생성 시 한 번 적용하는 검증 설정입니다.
type Option func(*configuration) error

// WithFieldTags는 공개 필드 이름을 선택할 struct 태그의 우선순위를 지정합니다.
// 기본은 json, form 순서이며 첫 번째로 존재하는 태그를 사용합니다.
func WithFieldTags(tags ...string) Option {
	tags = slices.Clone(tags)
	return func(c *configuration) error {
		if len(tags) == 0 {
			return invalidOption("field tags are empty")
		}
		seen := make(map[string]bool)
		for _, tag := range tags {
			if !validTagName(tag) || seen[tag] {
				return invalidOption("field tag is invalid or duplicated")
			}
			seen[tag] = true
		}
		c.fieldTags = slices.Clone(tags)
		return nil
	}
}

// WithValidationTag는 규칙을 읽을 struct 태그 이름을 바꿉니다. 기본은 validate입니다.
func WithValidationTag(tag string) Option {
	return func(c *configuration) error {
		if !validTagName(tag) {
			return invalidOption("validation tag is invalid")
		}
		c.validationTag = tag
		return nil
	}
}

// WithRule은 context를 받는 native 필드 규칙을 등록합니다.
// 규칙 함수는 동시 호출에 안전해야 하며 입력 DTO를 변경하지 않습니다.
func WithRule(name string, rule native.FuncCtx) Option {
	return func(c *configuration) error {
		if name == "" || rule == nil {
			return invalidOption("rule name and function are required")
		}
		c.rules = append(c.rules, func(v *native.Validate) error { return v.RegisterValidationCtx(name, rule) })
		return nil
	}
}

// WithStructRule은 여러 필드의 관계나 DTO 전체를 검사하는 native 규칙을 등록합니다.
// typed nil 포인터도 타입 표시에 사용할 수 있습니다. 실제 Validate의 nil DTO와는 다릅니다.
func WithStructRule(rule native.StructLevelFuncCtx, types ...any) Option {
	types = slices.Clone(types)
	return func(c *configuration) error {
		if rule == nil || len(types) == 0 {
			return invalidOption("struct rule and types are required")
		}
		registered := make([]any, len(types))
		for i, value := range types {
			t := reflect.TypeOf(value)
			if t == nil {
				return invalidOption("struct rule type is nil")
			}
			if t.Kind() == reflect.Pointer {
				t = t.Elem()
			}
			if t.Kind() != reflect.Struct {
				return invalidOption("struct rule requires a struct type")
			}
			// native 등록 함수는 typed nil을 역참조하므로 타입 표시를 빈 구조체로 바꿉니다.
			registered[i] = reflect.New(t).Elem().Interface()
		}
		c.rules = append(c.rules, func(v *native.Validate) error { v.RegisterStructValidationCtx(rule, registered...); return nil })
		return nil
	}
}

func invalidOption(message string) error {
	return fmt.Errorf("%w: %s", contract.ErrInvalidValidator, message)
}

func validTagName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if unicode.IsSpace(r) || unicode.IsControl(r) || r == ':' || r == '"' || r == '\\' {
			return false
		}
	}
	return true
}
