# spine-validator-playground

go-playground/validator를 사용하는 Spine DTO 검증 구현입니다. `github.com/NARUBROWN/spine/contract/validator.Validator`를 구현하며, 바인딩한 DTO의 `validate` 태그를 검사하고 필드별 오류를 공통 계약으로 변환합니다.

## Preview 설치

개발 중인 기능을 시험하고 호환성 문제를 확인하는 채널입니다. 공개 태그는 [`v0.2.0-preview.7`](https://github.com/NARUBROWN/spine-validator-playground/tree/v0.2.0-preview.7)입니다. 이 태그의 `go.mod`는 Spine `v0.7.0-preview.7`와 Go 1.26.0 이상을 지정합니다. 이 브랜치의 릴리스 정책 기준 버전은 `v0.2.0`입니다.

```sh
go get github.com/NARUBROWN/spine-validator-playground@v0.2.0-preview.7
```

다른 채널은 [RC](https://github.com/NARUBROWN/spine-validator-playground/tree/rc) | [Stable](https://github.com/NARUBROWN/spine-validator-playground/tree/main)에서 확인하세요.


정식 `v0.1.0`의 Spine `v0.6.5` 호환 범위는 아래에 별도로 설명합니다.

Go 1.26.0 이상과 go-playground/validator v10.30.6을 사용합니다. 공통 계약 식별자는 `spine.validation/v1`입니다.

## 앱에 연결

외부 구현의 Go 패키지 이름은 `playground`입니다.

```sh
go get github.com/NARUBROWN/spine-validator-playground@v0.2.0-preview.7
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

Spine이 HTTP JSON, 폼 DTO 또는 WebSocket, 컨슈머 메시지 DTO를 생성한 뒤 검증기를 호출합니다. 이 모듈은 DTO를 직접 바인딩하거나 HTTP 응답, WebSocket 종료, 메시지 ACK/NACK을 수행하지 않습니다. 이 전송 정책은 Spine이 소유합니다.

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
| 잘못된 옵션, 태그, 사용자 규칙의 패닉, 공개 경로 매핑 실패 | `contract.ErrInvalidValidator`로 감싼 오류 |
| context 취소, 시간 초과 | `errors.Is`로 원래 context 오류 확인 가능 |

오류의 `Field`는 `/items/0/email` 같은 JSON Pointer입니다. `~`와 `/`는 각각 `~0`, `~1`로 이스케이프합니다. Go 필드명, 중첩 구조체, 포인터, 배열, slice, map을 실제 DTO와 대조하여 공개 필드 경로로 변환합니다. 이름이 없는 익명 임베딩 구조체는 공개 필드를 평탄화하며, 명시적인 태그 이름이 있으면 그 이름 아래에 배치합니다.

map은 문자열, 정수, `encoding.TextMarshaler` 키를 지원합니다. 키에 점이나 대괄호가 있어도 실제 키와 대조합니다. 서로 다른 경로가 같은 native namespace를 만들거나 JSON에 대응할 수 없는 키라면 임의의 경로를 선택하지 않고 처리 오류를 반환합니다. `json:"-"`처럼 공개 입력에서 제외한 필드를 검증해 실패한 경우도 공개 경로가 없으므로 처리 오류입니다. 해당 필드를 검사하지 않으려면 `validate:"-"`를 지정합니다.

필드 오류는 공개 경로, 규칙, 설정값 순으로 정렬합니다. 거부된 입력값과 native 오류, 패닉의 원문은 공통 오류에 자동으로 복사하지 않습니다. `Param`은 태그나 사용자 규칙이 제공한 설정값입니다. 사용자 규칙도 여기에 입력값을 담지 않아야 합니다.

## 규칙과 옵션

| 옵션 | 동작 |
| --- | --- |
| `WithFieldTags("form", "json")` | 공개 이름 태그의 우선순위. 기본은 `json`, `form`, `query`, `header`. 첫 번째로 존재하는 태그를 사용하며 이름이 비어 있으면 Go 필드명을 사용 |
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

## 런타임 명세용 규칙 설명

`SchemaDescriber`가 포함된 Spine 코어와 함께 사용하면, 이 검증기는 실제 등록 설정에서 JSON Schema 제약을 제공합니다. 호출자는 같은 `playground.New(...)` 인스턴스를 `spine.WithValidator`에 연결하면 됩니다. 별도의 문서 태그나 검증기 등록은 필요하지 않습니다.

`DescribeSchema(reflect.Type)`는 `Validate`나 사용자 규칙을 실행하지 않습니다. `WithValidationTag`를 반영하며 `WithRule`로 덮어쓴 내장 규칙은 내장 제약으로 추론하지 않습니다. `WithStructRule`과 표현하지 못하는 규칙은 `Unresolved`에 남깁니다. 필드는 Go 인덱스로 연결하므로 JSON, 폼 이름은 코어의 바인딩 설명이 결정합니다.

지원 범위는 기본 required, 타입에 따른 min/max, gte/lte, gt/lt, len, oneof, format, 선두 omitempty/omitnil, 컨테이너 dive입니다. format은 일반적인 JSON Schema 형식으로 설명하며 native 검증기의 모든 문법과 판정을 재구현하지 않습니다. dive로 구조체 원소를 검증하면 원소 타입의 별도 설명이 필요하다는 진단을 남깁니다. 조건부, 필드 간, 사용자 정의 규칙과 복합 기본값은 완전히 변환하지 않습니다. 진단이 있는 설명은 부분 명세로 사용합니다.

숫자 required의 0 거부, bool required의 false 거부, 포인터 required의 null 거부를 각각 표현합니다. omitempty와 omitnil의 생략 분기는 값 제약과 별도로 설명합니다. 검증 제외 필드와 dive가 없는 컨테이너 내부에는 중첩 DTO 검증을 임의로 적용하지 않습니다.

개발 변경에서는 별도 쿼리와 헤더 구조체의 오류 필드 이름도 기본 태그 목록에서 선택합니다. 실제 바인딩 출처와 혼합 DTO 제한은 Spine 코어의 입력 규칙을 따릅니다.

## 요청별 필드 이름

Spine의 요청 컨텍스트에 `contract/validator.WithFieldTags`가 지정되면 이번 호출의 외부 이름에 그 태그 순서를 사용합니다. HTTP 바인딩은 json, form, xml, query 또는 header 중 실제 사용한 태그를 선택합니다. 지정하지 않은 호출은 생성 시 설정한 WithFieldTags 기본값을 유지합니다. 요청별 선택은 검증 규칙이나 공유 엔진의 설정을 변경하지 않습니다.

form을 선택한 호출에서는 태그 없는 중첩 값 구조체도 펼친 폼 필드의 오류 경로를 사용합니다. 고정 배열의 dive 제약은 처리할 위치를 prefixItems로 설명합니다. 길이 제약은 디코딩 후 Go 배열의 고정 길이를 기준으로 판단하고, 원소 제약이 기본값을 거부하면 부족한 요청 원소도 명세에서 거부합니다.

이 변경은 요청별 필드명 계약을 포함한 Spine 개발 버전과 함께 검증해야 합니다. 공개 배포 전에는 해당 코어 버전으로 의존성을 갱신해야 합니다.

## 타입 입력의 외부 필드 이름

요청별 `FieldNameMapper`는 외부 입력 이름을 검증 오류의 JSON Pointer에 사용합니다. `input.Path[T]`와 `input.Query[T]`가 태그 없이 정한 `userId` 같은 이름도 포함합니다. 실제 DTO 타입과 값, 공유 검증기 설정은 유지합니다.

명시적 source 태그와 익명 임베딩의 필드도 코어의 입력 계획을 따릅니다. 매핑이 없는 기존 호출은 기존 FieldTags 또는 검증기 기본 태그 규칙을 유지합니다. 설치 표에서 선택한 플랫폼 태그의 코어 의존성을 확인하세요.

## 릴리스와 공개 계약

이 플랫폼은 자체 버전과 채널로 발행합니다. `go.mod`의 Spine 버전은 사용하는 공개 계약의 최소 의존성이며 코어 릴리스 채널과 별도로 관리합니다. CI와 공개 설치 검증은 이 모듈의 의존성으로 플랫폼 자체를 검사합니다. 새로운 코어와 기존 플랫폼 태그의 조합 검증은 코어 릴리스에서 수행합니다.
