package playground_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	playground "github.com/NARUBROWN/spine-validator-playground"
	contract "github.com/NARUBROWN/spine/contract/validator"
	native "github.com/go-playground/validator/v10"
)

func newValidator(t *testing.T, options ...playground.Option) *playground.Validator {
	t.Helper()
	v, err := playground.New(options...)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

type field struct{ path, rule, param string }

func errorFields(t *testing.T, err error) []field {
	t.Helper()
	details, ok := contract.AsError(err)
	if !ok {
		t.Fatalf("공통 필드 오류여야 합니다: %v", err)
	}
	result := make([]field, len(details.Fields))
	for i, f := range details.Fields {
		result[i] = field{f.Field, f.Rule, f.Param}
	}
	return result
}

type UserDTO struct {
	Email    string `json:"email" validate:"required,email"`
	Age      int    `json:"age" validate:"gte=18,lte=120"`
	Role     string `json:"role" validate:"oneof=reader writer"`
	Optional *int   `json:"optional" validate:"omitempty,gte=0"`
}

type nilContext struct{ context.Context }

func TestValidatorStructsPointersAndRules(t *testing.T) {
	v := newValidator(t)
	good := UserDTO{Email: "kim@example.com", Age: 18, Role: "reader"}
	for _, value := range []any{good, &good} {
		if err := v.Validate(context.Background(), value); err != nil {
			t.Fatalf("정상 DTO는 통과해야 합니다: %v", err)
		}
	}
	bad := UserDTO{Email: "private-invalid-input", Age: 17, Role: "admin"}
	before := bad
	err := v.Validate(context.Background(), &bad)
	want := []field{{"/age", "gte", "18"}, {"/email", "email", ""}, {"/role", "oneof", "reader writer"}}
	if got := errorFields(t, err); !reflect.DeepEqual(got, want) {
		t.Fatalf("필드 오류=%+v", got)
	}
	if !reflect.DeepEqual(bad, before) {
		t.Fatal("입력 DTO를 변경하면 안 됩니다")
	}
	encoded, marshalErr := json.Marshal(err)
	if marshalErr != nil || strings.Contains(string(encoded), "private-invalid-input") {
		t.Fatalf("거부된 입력값을 오류로 복사하면 안 됩니다: %s %v", encoded, marshalErr)
	}
}

type Contact struct {
	Email string `json:"e.mail/~" validate:"email"`
}
type NestedDTO struct {
	Contacts []Contact           `json:"items" validate:"dive"`
	ByKey    map[string]*Contact `json:"by_key" validate:"dive"`
	Scores   map[string]int      `json:"scores" validate:"dive,gte=18"`
}

func TestNestedArraysMapsAndEscapedPublicNames(t *testing.T) {
	v := newValidator(t)
	dto := NestedDTO{
		Contacts: []Contact{{Email: "bad"}},
		ByKey:    map[string]*Contact{"a.b]/~[x]": {Email: "bad"}},
		Scores:   map[string]int{"second": 1, "first": 2},
	}
	want := []field{
		{"/by_key/a.b]~1~0[x]/e.mail~1~0", "email", ""},
		{"/items/0/e.mail~1~0", "email", ""},
		{"/scores/first", "gte", "18"},
		{"/scores/second", "gte", "18"},
	}
	for range 30 {
		if got := errorFields(t, v.Validate(context.Background(), dto)); !reflect.DeepEqual(got, want) {
			t.Fatalf("중첩 경로와 map 오류 순서가 달라졌습니다: %+v", got)
		}
	}
}

type Embedded struct {
	Name string `json:"display_name" validate:"required"`
}
type InlineDTO struct{ Embedded }
type NamedDTO struct {
	Embedded `json:"profile"`
}

func TestEmbeddedAndAnonymousDTOs(t *testing.T) {
	v := newValidator(t)
	for _, tt := range []struct {
		value any
		path  string
	}{
		{InlineDTO{}, "/display_name"}, {NamedDTO{}, "/profile/display_name"},
		{struct {
			Name string `json:"name" validate:"required"`
		}{}, "/name"},
	} {
		if got := errorFields(t, v.Validate(context.Background(), tt.value)); len(got) != 1 || got[0].path != tt.path {
			t.Fatalf("익명 구조체와 임베딩 경로를 유지해야 합니다: %+v", got)
		}
	}
}

func TestFormNamesAndCustomValidationTag(t *testing.T) {
	type DTO struct {
		Name string `json:"json_name" form:"form_name" check:"required"`
	}
	v := newValidator(t, playground.WithFieldTags("form", "json"), playground.WithValidationTag("check"))
	if got := errorFields(t, v.Validate(context.Background(), DTO{})); got[0].path != "/form_name" {
		t.Fatalf("선택한 태그 이름을 사용해야 합니다: %+v", got)
	}
	type Form struct {
		Name string `form:"name" validate:"required"`
	}
	if got := errorFields(t, newValidator(t).Validate(context.Background(), Form{})); got[0].path != "/name" {
		t.Fatalf("기본 설정에서도 form 태그를 지원해야 합니다: %+v", got)
	}
}

func TestInvalidInputConfigurationAndPanics(t *testing.T) {
	v := newValidator(t)
	for _, value := range []any{nil, (*UserDTO)(nil), 1, "input", map[string]string{}, []UserDTO{}, new(*UserDTO)} {
		err := v.Validate(context.Background(), value)
		if !errors.Is(err, contract.ErrInvalidValue) {
			t.Fatalf("지원하지 않는 입력은 호출 오류여야 합니다: %T %v", value, err)
		}
		if _, ok := contract.AsError(err); ok {
			t.Fatal("호출 오류를 필드 오류로 반환하면 안 됩니다")
		}
	}
	for _, value := range []any{
		struct {
			Name string `validate:"unknown_rule"`
		}{},
		struct {
			Name int `validate:"dive"`
		}{},
		struct {
			Name string `json:"-" validate:"required"`
		}{},
	} {
		if err := v.Validate(context.Background(), value); !errors.Is(err, contract.ErrInvalidValidator) {
			t.Fatalf("잘못된 규칙과 매핑은 구현 오류로 반환해야 합니다: %v", err)
		}
	}
	for _, invalid := range []*playground.Validator{nil, {}} {
		if err := invalid.Validate(context.Background(), UserDTO{}); !errors.Is(err, contract.ErrInvalidValidator) {
			t.Fatalf("초기화하지 않은 검증기를 거부해야 합니다: %v", err)
		}
	}
	if err := v.Validate(nil, UserDTO{}); !errors.Is(err, contract.ErrInvalidValidator) {
		t.Fatalf("nil context를 거부해야 합니다: %v", err)
	}
	if err := v.Validate((*nilContext)(nil), UserDTO{}); !errors.Is(err, contract.ErrInvalidValidator) {
		t.Fatalf("타입 정보가 있는 nil context도 거부해야 합니다: %v", err)
	}
	for _, options := range [][]playground.Option{
		{nil}, {playground.WithFieldTags()}, {playground.WithFieldTags("json", "json")},
		{playground.WithFieldTags("bad tag")}, {playground.WithValidationTag("")},
		{playground.WithFieldTags("bad\x00tag")},
		{playground.WithRule("custom", nil)},
		{playground.WithRule("bad,name", func(context.Context, native.FieldLevel) bool { return true })},
		{playground.WithStructRule(nil, UserDTO{})},
		{playground.WithStructRule(func(context.Context, native.StructLevel) {}, nil)},
		{playground.WithStructRule(func(context.Context, native.StructLevel) {}, 1)},
	} {
		if result, err := playground.New(options...); result != nil || !errors.Is(err, contract.ErrInvalidValidator) {
			t.Fatalf("잘못된 생성 옵션을 거부해야 합니다: %v", err)
		}
	}
}

func TestCustomRulesRootErrorsAndCancellation(t *testing.T) {
	type DTO struct {
		Name string `json:"name" validate:"request_rule"`
	}
	type key struct{}
	v := newValidator(t, playground.WithRule("request_rule", func(ctx context.Context, fl native.FieldLevel) bool {
		return ctx.Value(key{}) == "allowed" && fl.Field().String() == "Kim"
	}))
	ctx := context.WithValue(context.Background(), key{}, "allowed")
	if err := v.Validate(ctx, DTO{Name: "Kim"}); err != nil {
		t.Fatal(err)
	}
	if got := errorFields(t, v.Validate(context.Background(), DTO{Name: "Kim"})); got[0].rule != "request_rule" {
		t.Fatalf("사용자 규칙을 실행해야 합니다: %+v", got)
	}
	whole := newValidator(t, playground.WithStructRule(func(_ context.Context, sl native.StructLevel) {
		sl.ReportError(sl.Current().Interface(), "", "", "consistent", "")
	}, (*Embedded)(nil)))
	if got := errorFields(t, whole.Validate(context.Background(), Embedded{Name: "Kim"})); got[0].path != "" || got[0].rule != "consistent" {
		t.Fatalf("전체 DTO 오류는 빈 경로여야 합니다: %+v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.Validate(ctx, DTO{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("시작 전 취소를 보존해야 합니다: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	canceling := newValidator(t, playground.WithRule("request_rule", func(context.Context, native.FieldLevel) bool { cancel(); return false }))
	err := canceling.Validate(ctx, DTO{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("검증 중 취소를 보존해야 합니다: %v", err)
	}
	if _, ok := contract.AsError(err); ok {
		t.Fatal("취소를 입력 오류로 분류하면 안 됩니다")
	}
	panicking := newValidator(t, playground.WithRule("request_rule", func(context.Context, native.FieldLevel) bool { panic("private payload") }))
	err = panicking.Validate(context.Background(), DTO{})
	if !errors.Is(err, contract.ErrInvalidValidator) || strings.Contains(err.Error(), "private payload") {
		t.Fatalf("규칙 패닉을 원문 노출 없이 처리해야 합니다: %v", err)
	}
}

func TestRequiredPresenceSemantics(t *testing.T) {
	type DTO struct {
		Enabled *bool `json:"enabled" validate:"required"`
		Count   *int  `json:"count" validate:"required,gte=0"`
	}
	v := newValidator(t)
	no, zero := false, 0
	if err := v.Validate(context.Background(), DTO{Enabled: &no, Count: &zero}); err != nil {
		t.Fatalf("값을 보낸 false와 0은 포인터로 구분해야 합니다: %v", err)
	}
	if got := errorFields(t, v.Validate(context.Background(), DTO{})); len(got) != 2 {
		t.Fatalf("누락된 포인터는 required 실패여야 합니다: %+v", got)
	}
}

func TestConcurrentReuse(t *testing.T) {
	v := newValidator(t)
	var workers sync.WaitGroup
	for range 24 {
		workers.Go(func() {
			for range 20 {
				dto := UserDTO{Email: "kim@example.com", Age: 20, Role: "reader"}
				if err := v.Validate(context.Background(), &dto); err != nil {
					t.Errorf("공유 검증기의 정상 입력이 실패했습니다: %v", err)
				}
				if _, ok := contract.AsError(v.Validate(context.Background(), UserDTO{})); !ok {
					t.Error("공유 검증기의 필드 오류를 잃었습니다")
				}
			}
		})
	}
	workers.Wait()
}

func ExampleNew() {
	engine, err := playground.New()
	if err != nil {
		panic(err)
	}
	err = engine.Validate(context.Background(), UserDTO{Email: "bad", Age: 17, Role: "reader"})
	if details, ok := contract.AsError(err); ok {
		for _, field := range details.Fields {
			fmt.Println(field.Field, field.Rule)
		}
	}
	// Output:
	// /age gte
	// /email email
}

func TestDefaultFieldNamesIncludeQueryAndHeaderTags(t *testing.T) {
	v, err := playground.New()
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		value any
		field string
	}{
		{struct {
			Keyword string `query:"q" validate:"required"`
		}{}, "/q"},
		{struct {
			ID string `header:"X-Request-ID" validate:"required"`
		}{}, "/X-Request-ID"},
	} {
		err := v.Validate(context.Background(), test.value)
		var failure *contract.Error
		if !errors.As(err, &failure) || len(failure.Fields) != 1 || failure.Fields[0].Field != test.field {
			t.Fatalf("출처 태그의 검증 오류 경로: %v", err)
		}
	}
}

func TestRequestFieldNameMapperDoesNotChangeSharedValidator(t *testing.T) {
	type DTO struct {
		UserID int `validate:"required"`
		Alias  int `query:"UserID" validate:"required"`
	}
	validator := newValidator(t)
	before := errorFields(t, validator.Validate(context.Background(), DTO{}))
	ctx := contract.WithFieldTags(context.Background(), "query")
	ctx = contract.WithFieldNameMapper(ctx, func(owner reflect.Type, field reflect.StructField) (string, bool) {
		if owner != reflect.TypeFor[DTO]() {
			return "", false
		}
		if field.Name == "UserID" {
			return "userId", true
		}
		if field.Name == "Alias" {
			return "UserID", true
		}
		return "", false
	})
	fields := errorFields(t, validator.Validate(ctx, DTO{}))
	if len(fields) != 2 || fields[0].path != "/UserID" || fields[1].path != "/userId" {
		t.Fatal(fields)
	}
	original := errorFields(t, validator.Validate(context.Background(), DTO{}))
	if !reflect.DeepEqual(before, original) {
		t.Fatal(original)
	}
}
