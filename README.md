English | [Korean](README.ko.md)

# spine-validator-playground

Spine DTO validation using go-playground/validator. This module implements `github.com/NARUBROWN/spine/contract/validator.Validator`, checks bound DTO `validate` tags, and converts field errors to the shared contract.

## Preview installation

Use this channel to try features under development and check compatibility. The published tag is [`v0.2.0-preview.7`](https://github.com/NARUBROWN/spine-validator-playground/tree/v0.2.0-preview.7). Its `go.mod` specifies Spine `v0.7.0-preview.7` and Go 1.26.0 or later. The release policy on this branch targets `v0.2.0`.

```sh
go get github.com/NARUBROWN/spine-validator-playground@v0.2.0-preview.7
```

Other channels: [RC](https://github.com/NARUBROWN/spine-validator-playground/tree/rc) | [Stable](https://github.com/NARUBROWN/spine-validator-playground/tree/main).

See the v0.1.0 compatibility section below for the stable release's Spine `v0.6.5` compatibility.

Requires Go 1.26.0 or later and go-playground/validator v10.30.6. The shared contract identifier is `spine.validation/v1`.

## Application setup

The implementation's Go package name is `playground`.

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

Spine invokes the validator after creating DTOs for HTTP JSON, forms, WebSocket, and consumer messages. This module does not bind DTOs or produce HTTP responses, WebSocket closes, or message ACK/NACK. Spine owns those transport policies.

## Error handling

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

The error handling snippet belongs inside a function with the imports shown above. See `ExampleNew` in `validator_test.go` for a runnable example.

| Result | Contract |
| --- | --- |
| Valid DTO | nil |
| Rule violation | `*contract.Error`, with `Field`, `Rule`, `Param`, and a general description |
| Non-struct/non-struct-pointer value or nil DTO | Error wrapping `contract.ErrInvalidValue` |
| Invalid options/tags, panicking user rules, or failed public path mapping | Error wrapping `contract.ErrInvalidValidator` |
| Context cancellation or timeout | Original context error available through `errors.Is` |

An error's `Field` is a JSON Pointer such as `/items/0/email`. `~` and `/` are escaped as `~0` and `~1`. Go field names, nested structs, pointers, arrays, slices, and maps are checked against the actual DTO to produce public paths. Unnamed anonymous embedded structs flatten their public fields; explicit tag names create a nested path.

Maps support string, integer, and `encoding.TextMarshaler` keys. Keys containing dots or brackets are matched against actual keys. When different paths produce the same native namespace, or a key cannot map to JSON, validation returns a processing error instead of selecting an arbitrary path. A failed field excluded from public input with `json:"-"` also produces a processing error because it has no public path. Use `validate:"-"` to exclude it from validation.

Field errors are sorted by public path, rule, and parameter. Rejected input values, native errors, and panic text are not automatically copied into the shared error. `Param` comes from a tag or user rule; user rules must not place input values there.

## Rules and options

| Option | Behavior |
| --- | --- |
| `WithFieldTags("form", "json")` | Public name priority. Defaults to `json`, `form`, `query`, `header`. Uses the first existing tag; an empty name falls back to the Go field name. |
| `WithValidationTag("check")` | Tag containing rules; defaults to `validate` |
| `WithRule(name, native.FuncCtx)` | Register a custom field rule receiving context |
| `WithStructRule(native.StructLevelFuncCtx, types...)` | Register cross-field or whole-DTO rules; typed nil pointers can identify types |

Uses go-playground's default tags and built-in aliases. `WithRequiredStructEnabled` enables struct checks for `required`. Rule names and semantics follow the [official go-playground documentation](https://pkg.go.dev/github.com/go-playground/validator/v10). Numeric `required` rejects zero; ordinary bool `required` rejects false. To allow false or zero while rejecting missing fields, use `*bool` or `*int` to represent presence. The validator does not trim strings, transform input, or coerce types.

```go
engine, err := playground.New(playground.WithRule("company_code",
    func(ctx context.Context, field native.FieldLevel) bool {
        return strings.HasPrefix(field.Field().String(), "ACME-")
    },
))
```

The option example belongs inside a function importing `context`, `strings`, and `native "github.com/go-playground/validator/v10"`. Check `err` before using the validator. Struct rules must pass the actual Go field name as `ReportError`'s `structFieldName` so it can resolve public names. Report whole-DTO errors with `ReportError(value, "", "", rule, param)`. Synthetic nonexistent field paths are unsupported.

Complete configuration in `New`, then share the validator across requests. User rules must support concurrent calls, must not mutate DTOs or native engine settings, and must not retain input. Context is checked before and after validation and passed unchanged to user rules. Running user rules are not forcibly interrupted; long-running rules must respond to context cancellation.

## Verification

Verify against public dependencies only.

```sh
GOWORK=off go mod verify
GOWORK=off go vet ./...
GOWORK=off go test -race ./... -count=1
```

GitHub Actions checks the same public dependencies and fails if local replacements are present.

To compare with another local Spine worktree containing the contracts, pass its path.

```sh
bash scripts/verify-local.sh /path/to/spine
```

The script adds local replacements only to temporary module files and runs dependency integrity, formatting, `go vet`, and race checks. It preserves repository go.mod and go.sum files.

## License

MIT. See [LICENSE](LICENSE).

## v0.1.0 compatibility

This release requires Spine `v0.6.5` or later, which includes the public contracts. Install the official tag without a `replace` pointing to a private path.

```sh
go get github.com/NARUBROWN/spine-validator-playground@v0.1.0
```

## Rule descriptions for runtime specifications

With a Spine core providing `SchemaDescriber`, this validator supplies JSON Schema constraints from its actual registered configuration. Connect the same `playground.New(...)` instance through `spine.WithValidator`; separate documentation tags or validator registration are unnecessary.

`DescribeSchema(reflect.Type)` does not execute `Validate` or user rules. It respects `WithValidationTag` and does not infer built-in constraints for rules overridden through `WithRule`. `WithStructRule` and rules that cannot be expressed remain in `Unresolved`. Fields are linked by Go indexes, so the core's binding description determines JSON and form names.

Supported descriptions include basic required, type-dependent min/max, gte/lte, gt/lt, len, oneof, format, leading omitempty/omitnil, and container dive. Formats use common JSON Schema meanings without reimplementing every native validator grammar or decision. Dive over struct elements emits a diagnostic that the element type needs a separate description. Conditional, cross-field, custom rules, and compound defaults are not fully converted. Treat descriptions with diagnostics as partial specifications.

Descriptions distinguish numeric required rejecting zero, bool required rejecting false, and pointer required rejecting null. Omission branches for omitempty and omitnil are described separately from value constraints. Nested DTO validation is not inferred for excluded fields or inside containers without dive.

Development changes also select error names for separate query and header structs from the default tag list. Actual binding sources and mixed DTO restrictions follow the core's input rules.

## Per-request field names

When Spine's request context specifies `contract/validator.WithFieldTags`, that order selects external names for the call. HTTP binding selects the tag actually used: json, form, xml, query, or header. Calls without this setting retain the defaults configured by WithFieldTags at construction. Per-request selection does not change validation rules or shared engine settings.

When form is selected, untagged nested value structs also use flattened form field error paths. Fixed-array dive constraints use prefixItems to describe positions. Length constraints apply to the decoded Go array's fixed length. If element constraints reject default values, missing request elements are also rejected by the specification.

Verify these changes with a development Spine core containing the per-request field naming contract. Update the dependency to that core version before public publication.

## External field names for typed input

Per-request `FieldNameMapper` maps external input names to validation error JSON Pointers. This includes names such as `userId` selected by `input.Path[T]` and `input.Query[T]` without tags. Actual DTO types, values, and shared validator configuration remain unchanged.

Explicit source tags and anonymous embedded fields follow the core's input plan. Existing calls without a mapper retain their FieldTags or validator default naming rules. Check the selected platform tag's core dependency in the installation section.

## Releases and public contracts

This platform publishes its own versions and release channels. The Spine version in `go.mod` is the minimum dependency for its public contracts and is managed separately from the core release channel. CI and public installation checks verify the platform against this module's dependencies. Core releases verify combinations of a new core and existing platform tags.

The next publication target is `v0.2.1`.
