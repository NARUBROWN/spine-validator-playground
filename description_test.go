package playground

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	contract "github.com/NARUBROWN/spine/contract/validator"
	native "github.com/go-playground/validator/v10"
)

func TestDescriptionUsesRegisteredTagAndNeverExecutesCustomRules(t *testing.T) {
	calls := 0
	v, err := New(WithValidationTag("check"), WithRule("company", func(context.Context, native.FieldLevel) bool { calls++; return true }))
	if err != nil {
		t.Fatal(err)
	}
	type payload struct {
		Name    string `check:"required,min=2"`
		Age     int    `check:"gte=18,lte=120"`
		Role    string `check:"oneof=reader writer"`
		Custom  string `check:"company"`
		Ignored string `validate:"required"`
	}
	desc, err := v.DescribeSchema(reflect.TypeFor[payload]())
	if err != nil || calls != 0 {
		t.Fatalf("설명 중 사용자 규칙 실행: %d %v", calls, err)
	}
	if len(desc.Fields) != 4 || len(desc.Unresolved) != 1 || !strings.Contains(desc.Unresolved[0], "company") {
		t.Fatalf("설명: %+v", desc)
	}
	for _, i := range []int{0, 1, 2} {
		if !desc.Fields[i].Required {
			t.Fatalf("기본값이 규칙을 통과하지 않는 필드는 필수입니다: %+v", desc.Fields[i])
		}
	}
	encoded, _ := json.Marshal(desc)
	for _, token := range []string{`"minLength":2`, `"minimum":18`, `"maximum":120`, `"enum":["reader","writer"]`} {
		if !strings.Contains(string(encoded), token) {
			t.Fatalf("제약 누락 %s: %s", token, encoded)
		}
	}
}

func TestDescriptionDistinguishesPointersZeroValuesAndOmitEmpty(t *testing.T) {
	type payload struct {
		Number   int  `validate:"required"`
		Flag     bool `validate:"required"`
		Pointer  *int `validate:"required"`
		Optional int  `validate:"omitempty,gte=18"`
	}
	v, err := New()
	if err != nil {
		t.Fatal(err)
	}
	desc, err := v.DescribeSchema(reflect.TypeFor[payload]())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(desc)
	for _, token := range []string{`"const":0`, `"const":true`, `"type":"null"`, `"minimum":18`} {
		if !strings.Contains(string(data), token) {
			t.Fatalf("기본값과 null 제약 %s: %s", token, data)
		}
	}
	if desc.Fields[3].Required {
		t.Fatal("omitempty는 필수 입력이 아닙니다")
	}
	zero := 0
	if err := v.Validate(context.Background(), payload{Number: 1, Flag: true, Pointer: &zero}); err != nil {
		t.Fatal(err)
	}
	if err := v.Validate(context.Background(), payload{Flag: true, Pointer: &zero}); err == nil {
		t.Fatal("required 숫자는 0을 거부합니다")
	}
}

func TestCustomBuiltInOverrideIsNotDescribedAsBuiltIn(t *testing.T) {
	v, err := New(WithRule("email", func(context.Context, native.FieldLevel) bool { panic("설명에서 실행하면 안 됩니다") }))
	if err != nil {
		t.Fatal(err)
	}
	type payload struct {
		Email string `validate:"email"`
	}
	desc, err := v.DescribeSchema(reflect.TypeFor[payload]())
	if err != nil || len(desc.Unresolved) != 1 || strings.Contains(string(desc.Fields[0].Schema), "format") {
		t.Fatalf("덮어쓴 규칙: %+v %v", desc, err)
	}
	var _ contract.SchemaDescriber = v
}
