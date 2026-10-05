package calculus

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/nomagique/data"
)

func TestSignNext(t *testing.T) {
	Convey("Sign speaks only argument and sign", t, func() {
		mapping := data.NewMap(
			"argument", "input",
			"sign", "output",
		)

		for _, fixture := range []struct {
			input float64
			sign  float64
		}{
			{input: -2, sign: -1},
			{input: 0, sign: 0},
			{input: 3, sign: 1},
		} {
			value, ok := driveAdapter(NewSign(), mapping, map[string]float64{"input": fixture.input}, "output")
			So(ok, ShouldBeTrue)
			So(value, ShouldEqual, fixture.sign)
		}
	})
}
