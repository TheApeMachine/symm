package calculus_test

import (
	"errors"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/data"
	"testing"
)

func TestPositive(t *testing.T) {
	op := calculus.NewPositive()
	count := 0
	for range op.Next(data.NewValue(1.0, 2.0).Next(nil)) {
		count++
	}
	if count != 2 || op.Error() != nil {
		t.Fatal(count, op.Error())
	}
	for _, value := range []float64{0, -1} {
		op = calculus.NewPositive()
		count = 0
		for range op.Next(data.NewValue(1.0, value).Next(nil)) {
			count++
		}
		if count != 0 || !errors.Is(op.Error(), core.ErrDomain) {
			t.Fatal(count, op.Error())
		}
	}
}
