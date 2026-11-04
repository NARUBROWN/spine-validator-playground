package playground_test

import (
	"context"
	"reflect"
	"sync"
	"testing"

	contract "github.com/NARUBROWN/spine/contract/validator"
)

func TestRequestFieldNamesDoNotMutateTheSharedValidator(t *testing.T) {
	type input struct {
		Name string `json:"json_name" form:"form_name" query:"q" header:"X-Name" validate:"required"`
	}
	v := newValidator(t)
	for _, test := range []struct{ tag, path string }{{"json", "/json_name"}, {"form", "/form_name"}, {"query", "/q"}, {"header", "/X-Name"}} {
		err := v.Validate(contract.WithFieldTags(context.Background(), test.tag), input{})
		if got := errorFields(t, err); !reflect.DeepEqual(got, []field{{test.path, "required", ""}}) {
			t.Fatalf("바인딩한 입력의 이름을 사용해야 합니다: %+v", got)
		}
	}
	if got := errorFields(t, v.Validate(context.Background(), input{})); got[0].path != "/json_name" {
		t.Fatal("공유 검증기의 기본 이름을 변경하면 안 됩니다")
	}
	var group sync.WaitGroup
	for _, tag := range []string{"json", "form", "query", "header"} {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 50 {
				_ = v.Validate(contract.WithFieldTags(context.Background(), tag), input{})
			}
		}()
	}
	group.Wait()
}
