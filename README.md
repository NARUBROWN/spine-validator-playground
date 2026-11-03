# spine-validator-playground

go-playground/validator를 사용하는 Spine DTO 검증 구현입니다. `github.com/NARUBROWN/spine/contract/validator.Validator`를 구현하며, 바인딩한 DTO의 `validate` 태그를 검사하고 필드별 오류를 공통 계약으로 변환합니다.

Spine `v0.6.5`의 `contract/validator`와 `spine.WithValidator`를 사용합니다. 로컬 `replace` 없이 정식 태그에 의존합니다. 기존 v0.6.0 앱은 코어 의존성과 생성식을 함께 갱신하세요.

Go 1.26.0 이상과 go-playground/validator v10.30.6을 사용합니다. 공통 계약 식별자는 `spine.validation/v1`입니다.

## 앱에 연결

외부 구현의 Go 패키지 이름은 `playground`입니다.

```sh
go get github.com/NARUBROWN/spine-validator-playground
```

```go
import (
    "github.com/NARUBROWN/spine"
    platform "github.com/NARUBROWN/spine-echo-platform"
    playground "github.com/NARUBROWN/spine-validator-playground"
)

func newApplication() (spine.App, error) {
    engine, err := playground.New()
    if err != nil {
        return nil, err
    }
    return spine.New(platform.Option(), spine.WithValidator(engine)), nil
}
```

```go
type CreateUserDTO struct {
    Email string `json:"email" validate:"required,email"`
    Age   int    `json:"age" validate:"gte=18,lte=120"`
    Role  string `json:"role" validate:"oneof=reader writer"`
}
```

Spine이 HTTP JSON·폼 DTO 또는 WebSocket·컨슈머 메시지 DTO를 생성한 뒤 검증기를 호출합니다. 이 모듈은 DTO를 직접 바인딩하거나 HTTP 응답·WebSocket 종료·메시지 ACK/NACK을 수행하지 않습니다. 이 전송 정책은 Spine이 소유합니다.

## 오류 처리

```go
import (
    "context"
    contract "github.com/NARUBROWN/spine/contract/validator"
)

err := engine.Validate(context.Background(), CreateUserDTO{
    Email: "invalid",
    Age:   17,
    Role:  "reader",
})
if failure, ok := contract.AsError(err); ok {
    // /age: gte, param=18
    // /email: email
    _ = failure.Fields
}
```

오류 처리 코드는 위 import를 사용하는 함수 내부의 예입니다. 전체 실행 예는 `validator_test.go`의 `ExampleNew`에서 확인할 수 있습니다.

| 결과 | 계약 |
| --- | --- |
| 정상 DTO | nil |
| 규칙 위반 | `*contract.Error`. `Field`, `Rule`, `Param`, 일반 설명을 제공 |
| 구조체나 구조체 포인터가 아닌 값, nil DTO | `contract.ErrInvalidValue`로 감싼 오류 |
| 잘못된 옵션·태그, 사용자 규칙의 패닉, 공개 경로 매핑 실패 | `contract.ErrInvalidValidator`로 감싼 오류 |
| context 취소·시간 초과 | `errors.Is`로 원래 context 오류 확인 가능 |

오류의 `Field`는 `/items/0/email` 같은 JSON Pointer입니다. `~`와 `/`는 각각 `~0`, `~1`로 이스케이프합니다. Go 필드명, 중첩 구조체·포인터·배열·slice·map을 실제 DTO와 대조하여 공개 필드 경로로 변환합니다. 이름이 없는 익명 임베딩 구조체는 공개 필드를 평탄화하며, 명시적인 태그 이름이 있으면 그 이름 아래에 배치합니다.

map은 문자열·정수·`encoding.TextMarshaler` 키를 지원합니다. 키에 점이나 대괄호가 있어도 실제 키와 대조합니다. 서로 다른 경로가 같은 native namespace를 만들거나 JSON에 대응할 수 없는 키라면 임의의 경로를 선택하지 않고 처리 오류를 반환합니다. `json:"-"`처럼 공개 입력에서 제외한 필드를 검증해 실패한 경우도 공개 경로가 없으므로 처리 오류입니다. 해당 필드를 검사하지 않으려면 `validate:"-"`를 지정합니다.

필드 오류는 공개 경로·규칙·설정값 순으로 정렬합니다. 거부된 입력값과 native 오류·패닉의 원문은 공통 오류에 자동으로 복사하지 않습니다. `Param`은 태그나 사용자 규칙이 제공한 설정값입니다. 사용자 규칙도 여기에 입력값을 담지 않아야 합니다.

## 규칙과 옵션

| 옵션 | 동작 |
| --- | --- |
| `WithFieldTags("form", "json")` | 공개 이름 태그의 우선순위. 기본은 `json`, `form`. 첫 번째로 존재하는 태그를 사용하며 이름이 비어 있으면 Go 필드명을 사용 |
| `WithValidationTag("check")` | 규칙을 읽을 태그 이름. 기본은 `validate` |
| `WithRule(name, native.FuncCtx)` | context를 받는 사용자 필드 규칙 등록 |
| `WithStructRule(native.StructLevelFuncCtx, types...)` | 필드 간 관계와 DTO 전체 규칙 등록. typed nil 포인터는 타입 표시로 사용 가능 |

go-playground의 기본 태그와 내장 alias를 사용합니다. `required`의 구조체 검사에는 `WithRequiredStructEnabled`를 적용합니다. 규칙 이름과 판정은 [go-playground 공식 문서](https://pkg.go.dev/github.com/go-playground/validator/v10)를 따릅니다. 숫자의 `required`는 0, 일반 bool의 `required`는 false도 거부합니다. false와 0을 허용하면서 누락만 거부하려면 `*bool`, `*int` 등으로 필드 존재 여부를 구분하세요. 문자열 앞뒤 공백 제거, 입력값 변환과 타입 강제 변환은 하지 않습니다.

```go
engine, err := playground.New(playground.WithRule("company_code",
    func(ctx context.Context, field native.FieldLevel) bool {
        return strings.HasPrefix(field.Field().String(), "ACME-")
    },
))
```

이 옵션 예는 `context`, `strings`, `native "github.com/go-playground/validator/v10"`을 import한 함수 내부 코드입니다. `err`를 확인한 뒤 검증기를 사용합니다. 구조체 규칙은 `ReportError`의 `structFieldName`에 실제 Go 필드 이름을 전달해야 공개 이름을 해석할 수 있습니다. DTO 전체 오류는 `ReportError(value, "", "", rule, param)`으로 보고합니다. 존재하지 않는 합성 필드 경로는 지원하지 않습니다.

모든 설정은 `New`에서 마치며 완성된 검증기를 여러 요청에서 공유합니다. 사용자 규칙은 동시 호출에 안전해야 하고 DTO나 native 엔진 설정을 변경하거나 입력을 보관하면 안 됩니다. context는 검증 시작 전과 종료 시 확인하며 사용자 규칙에 그대로 전달합니다. 실행 중인 사용자 규칙을 강제로 중단하지 않으므로 긴 작업은 규칙 함수가 context 취소에 응답해야 합니다.

## 검증

공개 의존성만으로 검증합니다.

```sh
GOWORK=off go mod verify
GOWORK=off go vet ./...
GOWORK=off go test -race ./... -count=1
```

GitHub Actions도 이 공개 의존성을 검사하며 로컬 replace가 있으면 실패합니다.

계약이 포함된 다른 로컬 Spine 작업 트리와 비교할 때는 경로를 전달합니다.

```sh
bash scripts/verify-local.sh /path/to/spine
```

스크립트는 임시 모듈 파일에만 로컬 replace를 추가하고 의존성 무결성, 서식, `go vet`, race 테스트를 실행합니다. 저장소의 go.mod와 go.sum은 변경하지 않습니다.

## 라이선스

MIT. [LICENSE](LICENSE)를 참고하세요.

## v0.1.0 호환 버전

이 릴리스는 공개 계약을 포함한 Spine `v0.6.5` 이상을 요구합니다. 개인 경로의 `replace` 없이 정식 태그로 설치합니다.

```sh
go get github.com/NARUBROWN/spine-validator-playground@v0.1.0
```
