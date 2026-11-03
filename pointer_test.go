package playground_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	playground "github.com/NARUBROWN/spine-validator-playground"
	contract "github.com/NARUBROWN/spine/contract/validator"
)

type textKey int

func (k textKey) MarshalText() ([]byte, error) { return []byte("key/~"), nil }

func TestMapKeysAndAmbiguousNativeNamespace(t *testing.T) {
	v := newValidator(t)
	for _, tt := range []struct {
		value any
		path  string
	}{
		{struct {
			Values map[int]int `json:"values" validate:"dive,gte=1"`
		}{Values: map[int]int{7: 0}}, "/values/7"},
		{struct {
			Values map[textKey]int `json:"values" validate:"dive,gte=1"`
		}{Values: map[textKey]int{7: 0}}, "/values/key~1~0"},
		{struct {
			Values [1]string `json:"values" validate:"dive,required"`
		}{}, "/values/0"},
		{struct {
			Values map[string]any `json:"values" validate:"dive,required"`
		}{Values: map[string]any{"": nil}}, "/values/"},
	} {
		if got := errorFields(t, v.Validate(context.Background(), tt.value)); len(got) != 1 || got[0].path != tt.path {
			t.Fatalf("map 키와 배열 경로를 보존해야 합니다: %+v", got)
		}
	}
	dto := struct {
		Values map[string]any `json:"values" validate:"dive,required"`
	}{
		Values: map[string]any{"a": map[string]string{"b": ""}, "a][b": nil},
	}
	if err := v.Validate(context.Background(), dto); !errors.Is(err, contract.ErrInvalidValidator) {
		t.Fatalf("둘 이상의 공개 경로에 대응하는 namespace를 임의로 선택하면 안 됩니다: %v", err)
	}
}

func TestPointerOptionalFieldsAndEmbeddedPointers(t *testing.T) {
	v := newValidator(t)
	type DTO struct {
		Contact *Contact `json:"contact" validate:"required"`
	}
	if got := errorFields(t, v.Validate(context.Background(), DTO{})); !reflect.DeepEqual(got, []field{{"/contact", "required", ""}}) {
		t.Fatalf("nil 포인터 필드의 경로를 유지해야 합니다: %+v", got)
	}
	type PointerInline struct{ *Embedded }
	if got := errorFields(t, v.Validate(context.Background(), PointerInline{Embedded: &Embedded{}})); got[0].path != "/display_name" {
		t.Fatalf("임베딩 포인터도 공개 필드 경로를 사용해야 합니다: %+v", got)
	}
}

func FuzzStringMapPointer(f *testing.F) {
	for _, key := range []string{"", "ordinary", "a/b~c", "a.b][c", "한글", "[0]", "a\x00b"} {
		f.Add(key)
	}
	v, err := playground.New()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, key string) {
		if !utf8.ValidString(key) {
			t.Skip()
		}
		dto := struct {
			Values map[string]string `json:"values" validate:"dive,required"`
		}{Values: map[string]string{key: ""}}
		got := errorFields(t, v.Validate(context.Background(), dto))
		want := "/values/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
		if len(got) != 1 || got[0].path != want {
			t.Fatalf("map 키를 정확히 한 번 이스케이프해야 합니다: %+v 기대값=%q", got, want)
		}
	})
}
