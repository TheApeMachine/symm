package temporal_test

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/temporal"
	"testing"
)

func TestPrevious(t *testing.T) {
	op := temporal.NewPrevious()
	for range op.Next(data.NewValue(1.0, 2.0).Next(nil)) {
		t.Fatal("invented prior")
	}
	for range op.Next(data.NewValue[float64]().Next(nil)) {
		t.Fatal("empty update")
	}
	var got []float64
	for pointer := range op.Next(data.NewValue(3.0, 4.0).Next(nil)) {
		group := *(*core.Primitive)(pointer)
		for value := range group.Next(nil) {
			got = append(got, *(*float64)(value))
		}
	}
	if len(got) != 4 {
		t.Fatal(got)
	}
	for index, value := range got {
		if value != float64(index+1) {
			t.Fatal(got)
		}
	}
}
