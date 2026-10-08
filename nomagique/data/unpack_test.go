package data_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
)

func TestUnpack(t *testing.T) {
	op := data.NewUnpack()
	index := 0
	for pointer := range op.Next(data.NewValue[core.Primitive](data.NewValue(1.0, 2.0), data.NewValue(3.0)).Next(nil)) {
		index++
		if *(*float64)(pointer) != float64(index) {
			t.Fatal("operand changed")
		}
	}
	if index != 3 || op.Error() != nil {
		t.Fatalf("count=%d error=%v", index, op.Error())
	}
}
