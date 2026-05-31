package perspectives

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGaugeFactorsFrom(t *testing.T) {
	Convey("Given a wire row with named metrics", t, func() {
		factors := GaugeFactorsFrom(map[string]any{
			"div":     -0.2,
			"re":      1.5,
			"missing": "not-a-float",
		}, "div", "re", "missing")

		Convey("It should extract numeric factors", func() {
			So(len(factors), ShouldEqual, 2)
			So(factors[0].Name, ShouldEqual, "div")
			So(factors[0].Value, ShouldEqual, -0.2)
			So(factors[1].Name, ShouldEqual, "re")
		})
	})
}

func BenchmarkGaugeFactorsFrom(b *testing.B) {
	row := map[string]any{
		"div":     0.2,
		"vort":    0.4,
		"turb_fd": 1.1,
		"re":      1.5,
		"visc":    0.8,
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = GaugeFactorsFrom(row, "div", "vort", "turb_fd", "re", "visc")
	}
}
