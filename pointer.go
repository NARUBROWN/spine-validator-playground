package playground

import (
	"encoding"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	contract "github.com/NARUBROWN/spine/contract/validator"
)

type fieldMapper struct {
	root reflect.Value
	tags []string
	maps map[mapIdentity]map[string][]reflect.Value
}

type mapIdentity struct {
	address uintptr
	typeOf  reflect.Type
}

func (m *fieldMapper) fieldPointer(namespace string) (string, error) {
	value := indirect(m.root)
	name := value.Type().Name()
	if name != "" {
		prefix := name + "."
		if !strings.HasPrefix(namespace, prefix) {
			return "", invalidPath()
		}
		namespace = strings.TrimPrefix(namespace, prefix)
	}
	paths, err := m.resolvePaths(value, namespace, nil)
	if err != nil || len(paths) != 1 {
		return "", invalidPath()
	}
	return paths[0], nil
}

// 원본 namespace의 map 키를 구분자로 나누지 않고 실제 키와 대조합니다.
// 서로 다른 Go 경로가 같은 namespace를 만들면 잘못된 필드 경로를 반환하지 않습니다.
func (m *fieldMapper) resolvePaths(value reflect.Value, namespace string, tokens []string) ([]string, error) {
	if namespace == "" {
		return []string{jsonPointer(tokens)}, nil
	}
	value = indirect(value)
	if !value.IsValid() {
		return nil, nil
	}
	switch value.Kind() {
	case reflect.Struct:
		name := namespace
		if i := strings.IndexAny(name, ".["); i >= 0 {
			name = name[:i]
		}
		field, ok := value.Type().FieldByName(name)
		if !ok {
			return nil, nil
		}
		current := value
		next := slicesOf(tokens)
		for _, index := range field.Index {
			current = indirect(current)
			if !current.IsValid() || current.Kind() != reflect.Struct {
				return nil, nil
			}
			meta := current.Type().Field(index)
			name, flatten, err := publicName(meta, m.tags)
			if err != nil {
				return nil, err
			}
			if !flatten {
				next = append(next, name)
			}
			current = current.Field(index)
		}
		rest := strings.TrimPrefix(namespace, field.Name)
		rest = strings.TrimPrefix(rest, ".")
		return m.resolvePaths(current, rest, next)
	case reflect.Array, reflect.Slice:
		if !strings.HasPrefix(namespace, "[") {
			return nil, nil
		}
		end := strings.IndexByte(namespace, ']')
		if end < 0 {
			return nil, nil
		}
		index, err := strconv.Atoi(namespace[1:end])
		if err != nil || index < 0 || index >= value.Len() {
			return nil, nil
		}
		rest := strings.TrimPrefix(namespace[end+1:], ".")
		return m.resolvePaths(value.Index(index), rest, appendToken(tokens, strconv.Itoa(index)))
	case reflect.Map:
		var paths []string
		index := m.mapIndex(value)
		// 키에 ']'가 있을 수 있으므로 가능한 모든 닫는 위치를 사전과 대조합니다.
		for end := 0; end < len(namespace); end++ {
			if namespace[end] != ']' {
				continue
			}
			keys := index[namespace[:end+1]]
			if len(keys) == 0 {
				continue
			}
			rest := namespace[end+1:]
			if rest != "" && rest[0] != '.' && rest[0] != '[' {
				continue
			}
			for _, key := range keys {
				name, err := mapKeyName(key)
				if err != nil {
					return nil, err
				}
				matches, err := m.resolvePaths(value.MapIndex(key), strings.TrimPrefix(rest, "."), appendToken(tokens, name))
				if err != nil {
					return nil, err
				}
				paths = append(paths, matches...)
			}
		}
		return paths, nil
	}
	return nil, nil
}

// 한 번의 Validate 안에서만 map 키를 색인하여 오류마다 전체 map을 재순회하지 않습니다.
// 주소는 살아 있는 map의 식별자로만 쓰며 역참조하지 않습니다. DTO와 색인은 반환 후 보관하지 않습니다.
func (m *fieldMapper) mapIndex(value reflect.Value) map[string][]reflect.Value {
	if m.maps == nil {
		m.maps = make(map[mapIdentity]map[string][]reflect.Value)
	}
	identity := mapIdentity{address: uintptr(value.UnsafePointer()), typeOf: value.Type()}
	if index, ok := m.maps[identity]; ok {
		return index
	}
	index := make(map[string][]reflect.Value, value.Len())
	for _, key := range value.MapKeys() {
		prefix := "[" + fmt.Sprintf("%v", key) + "]"
		index[prefix] = append(index[prefix], key)
	}
	m.maps[identity] = index
	return index
}

func publicName(field reflect.StructField, tags []string) (name string, flatten bool, err error) {
	for _, tag := range tags {
		if value, ok := field.Tag.Lookup(tag); ok {
			name, _, _ = strings.Cut(value, ",")
			if name == "-" {
				return "", false, invalidPath()
			}
			break
		}
	}
	if name != "" {
		return name, false, nil
	}
	t := field.Type
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return field.Name, field.Anonymous && t.Kind() == reflect.Struct, nil
}

func mapKeyName(key reflect.Value) (string, error) {
	if key.Kind() == reflect.String {
		return key.String(), nil
	}
	if key.CanInterface() {
		if marshaler, ok := key.Interface().(encoding.TextMarshaler); ok {
			text, err := marshaler.MarshalText()
			if err != nil {
				return "", invalidPath()
			}
			return string(text), nil
		}
	}
	switch key.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return strconv.FormatInt(key.Int(), 10), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return strconv.FormatUint(key.Uint(), 10), nil
	}
	return "", invalidPath()
}

func indirect(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}
		value = value.Elem()
	}
	return value
}

func jsonPointer(tokens []string) string {
	var result strings.Builder
	for _, token := range tokens {
		result.WriteByte('/')
		result.WriteString(strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1"))
	}
	return result.String()
}

func slicesOf(tokens []string) []string                  { return append([]string(nil), tokens...) }
func appendToken(tokens []string, token string) []string { return append(slicesOf(tokens), token) }
func invalidPath() error {
	return fmt.Errorf("%w: cannot resolve a unique public field path", contract.ErrInvalidValidator)
}
