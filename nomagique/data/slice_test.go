package data_test

import (
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
)

func TestSlice(t *testing.T) {
	for _, end := range [][]int{nil, {4}} {
		op := data.NewSlice(2, end...)
		index := 2
		for pointer := range op.Next(data.NewValue(0.0, 1.0, 2.0, 3.0).Next(nil)) {
			if *(*float64)(pointer) != float64(index) {
				t.Fatal("wrong slice")
			}
			index++
		}
		if index != 4 {
			t.Fatal(index)
		}
	}
}
