package calculus

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/tests"
	"github.com/theapemachine/symm/nomagique/transport"
	"testing"
)

func TestConvertNext(t *testing.T) {
	result := tests.Drain(t, NewConvert[int64, float64](), transport.NewIO(core.From(int64(12))))
	tests.EqualNumber(t, result[0], 12)
}
