package tests

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

// Values and Record are shared boundary fixtures, not alternate execution.
func Values[T any](values ...T) core.Primitive {
	entries := make([]core.Primitive, len(values))
	for index, value := range values {
		entries[index] = core.From(value)
	}
	return transport.NewIO(entries...)
}
func Record(fields map[string]any) core.Primitive {
	entries := make(map[string]core.Primitive, len(fields))
	for key, value := range fields {
		entries[key] = core.From(value)
	}
	return core.From(entries)
}
func Fields(t *testing.T, value any) map[string]core.Primitive {
	t.Helper()
	fields, ok := value.(map[string]core.Primitive)
	if !ok {
		t.Fatalf("expected record, received %T", value)
	}
	return fields
}
func Number(t *testing.T, fields map[string]core.Primitive, key string) float64 {
	t.Helper()
	value, ok := fields[key]
	if !ok {
		t.Fatalf("missing field %q", key)
	}
	result := core.To[float64](value)
	if err := value.Error(); err != nil {
		t.Fatal(err)
	}
	return result
}
func Sound(t *testing.T, node core.Primitive) {
	t.Helper()
	if err := node.Error(); err != nil {
		t.Fatal(err)
	}
}
