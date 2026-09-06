package tests

import (
	"errors"
	"testing"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/transport"
)

// CheckTypeFailure is shared by consumers; it does not implement a fake fold.
func CheckTypeFailure(t *testing.T, operation core.Primitive) {
	t.Helper()
	input := transport.NewIO(core.From("not a number"))
	result := operation.Next(input)
	if result == nil || !errors.Is(result.Error(), core.ErrWrongType) {
		t.Fatal("the yielded result lost its input type error")
	}
	if operation.Next(input) != nil {
		t.Fatal("failed reduction did not end its delivery run")
	}
	if !errors.Is(operation.Error(), core.ErrWrongType) {
		t.Fatal("the operation lost the error at the terminating nil")
	}
}
