package playground

import (
	"reflect"
	"strings"
	"testing"
)

func TestDescribeConditionalRequired(t *testing.T) {
	type action struct {
		Operation string
		State     string
		ID        string `validate:"required_if=Operation 'undo now' State ready"`
		Optional  string `validate:"omitempty,required_if=Operation 'undo now'"`
		Unknown   string `validate:"required_if=Operation ''"`
	}
	v, err := New()
	if err != nil {
		t.Fatal(err)
	}
	description, err := v.DescribeSchema(reflect.TypeFor[action]())
	if err != nil || len(description.ConditionalRequired) != 1 {
		t.Fatalf("description=%+v err=%v", description, err)
	}
	rule := description.ConditionalRequired[0]
	if !reflect.DeepEqual(rule.Index, []int{2}) || string(rule.Schema) != `{"minLength":1}` || len(rule.When) != 2 ||
		!reflect.DeepEqual(rule.When[0].Index, []int{0}) || string(rule.When[0].Value) != `"undo now"` ||
		!reflect.DeepEqual(rule.When[1].Index, []int{1}) || string(rule.When[1].Value) != `"ready"` {
		t.Fatalf("rule=%+v", rule)
	}
	if !strings.Contains(strings.Join(description.Unresolved, "\n"), "Optional: required_if") ||
		!strings.Contains(strings.Join(description.Unresolved, "\n"), "Unknown: required_if") {
		t.Fatalf("unresolved=%v", description.Unresolved)
	}
	for _, field := range description.Fields {
		if field.Required {
			t.Fatalf("unexpected unconditional requirement: %+v", field)
		}
	}
}
