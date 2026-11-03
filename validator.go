// Package playground는 go-playground/validator를 사용하는 Spine DTO 검증 구현입니다.
package playground

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"

	contract "github.com/NARUBROWN/spine/contract/validator"
	native "github.com/go-playground/validator/v10"
)

// Validator는 생성 시 규칙 등록을 마친 뒤 여러 요청에서 공유하는 검증기입니다.
// 입력 DTO를 변경하지 않으며, 생성 후 설정을 변경하는 메서드는 제공하지 않습니다.
type Validator struct {
	engine        *native.Validate
	fieldTags     []string
	validationTag string
	customRules   map[string]bool
	structRules   map[reflect.Type]bool
}

var _ contract.Validator = (*Validator)(nil)

// New는 검증 태그를 해석할 엔진을 준비하고 모든 사용자 규칙을 등록합니다.
// 잘못된 옵션은 ErrInvalidValidator로 감싼 오류와 nil 검증기를 반환합니다.
// DTO의 태그 오류는 해당 DTO를 검증할 때 같은 처리 오류로 반환합니다.
func New(options ...Option) (result *Validator, err error) {
	defer func() {
		if recover() != nil {
			result = nil
			err = fmt.Errorf("%w: rule configuration panicked", contract.ErrInvalidValidator)
		}
	}()
	config := configuration{fieldTags: []string{"json", "form"}, validationTag: "validate"}
	for _, option := range options {
		if option == nil {
			return nil, fmt.Errorf("%w: option is nil", contract.ErrInvalidValidator)
		}
		if err := option(&config); err != nil {
			return nil, err
		}
	}
	engine := native.New(native.WithRequiredStructEnabled())
	engine.SetTagName(config.validationTag)
	for _, configure := range config.rules {
		if err := configure(engine); err != nil {
			return nil, fmt.Errorf("%w: rule registration failed", contract.ErrInvalidValidator)
		}
	}
	return &Validator{engine: engine, fieldTags: config.fieldTags, validationTag: config.validationTag, customRules: config.customRules, structRules: config.structRules}, nil
}

// Validate는 바인딩된 DTO의 태그와 사용자 규칙을 실행합니다.
// 필드 오류는 입력값을 복사하지 않고 공통 JSON Pointer와 규칙 정보로 변환합니다.
func (v *Validator) Validate(ctx context.Context, value any) (err error) {
	if v == nil || v.engine == nil {
		return fmt.Errorf("%w: validator is not initialized", contract.ErrInvalidValidator)
	}
	if nilContext(ctx) {
		return fmt.Errorf("%w: context is nil", contract.ErrInvalidValidator)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("%w: validation engine panicked", contract.ErrInvalidValidator)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			err = errors.Join(ctxErr, err)
		}
	}()
	root := reflect.ValueOf(value)
	if !root.IsValid() || (root.Kind() != reflect.Struct &&
		(root.Kind() != reflect.Pointer || root.IsNil() || root.Type().Elem().Kind() != reflect.Struct)) {
		return fmt.Errorf("%w: expected struct or non-nil struct pointer", contract.ErrInvalidValue)
	}
	err = v.engine.StructCtx(ctx, value)
	if err == nil {
		return nil
	}
	var failures native.ValidationErrors
	if !errors.As(err, &failures) {
		return fmt.Errorf("%w: validation engine failed", contract.ErrInvalidValidator)
	}
	fields := make([]contract.FieldError, 0, len(failures))
	mapper := fieldMapper{root: root, tags: v.fieldTags}
	for _, failure := range failures {
		if failure.Tag() == "" {
			return fmt.Errorf("%w: validation rule is empty", contract.ErrInvalidValidator)
		}
		pointer, pathErr := mapper.fieldPointer(failure.StructNamespace())
		if pathErr != nil {
			return pathErr
		}
		fields = append(fields, contract.FieldError{
			Field: pointer, Rule: failure.Tag(), Param: failure.Param(),
			Message: "검증 규칙을 만족하지 않습니다.",
		})
	}
	// native 엔진의 map 순회 순서와 관계없이 같은 오류 순서를 제공합니다.
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Field != fields[j].Field {
			return fields[i].Field < fields[j].Field
		}
		if fields[i].Rule != fields[j].Rule {
			return fields[i].Rule < fields[j].Rule
		}
		return fields[i].Param < fields[j].Param
	})
	return contract.NewError(fields...)
}

func nilContext(ctx context.Context) bool {
	if ctx == nil {
		return true
	}
	value := reflect.ValueOf(ctx)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	}
	return false
}
