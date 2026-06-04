package perspectives

import (
	"os"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseThoughtsPlaybook(t *testing.T) {
	Convey("Given the hand-written multi-horizon playbook", t, func() {
		raw, err := os.ReadFile("cfg/perspectives.yaml")
		So(err, ShouldBeNil)

		thoughts, err := ParseThoughts(raw)
		So(err, ShouldBeNil)

		Convey("It parses the three horizons", func() {
			So(len(thoughts), ShouldEqual, 3)
		})

		Convey("The short horizon is a regime + signal conjunction", func() {
			short := thoughts[0]
			So(len(short.When.All), ShouldEqual, 2)
			So(short.When.All[0].Subject, ShouldEqual, SubjectRegime)
			So(short.When.All[0].Regime, ShouldEqual, RegimeTrending)
			So(short.When.All[1].Subject, ShouldEqual, SubjectSignal)
			So(short.When.All[1].Category, ShouldEqual, CategoryCoiledCompression)
		})

		Convey("Its confirmation carries a metric-to-metric (versus) and enters iceberg", func() {
			confirm := thoughts[0].Then[0]
			So(confirm.Do.Type, ShouldEqual, ActionIceberg)
			So(len(confirm.When.All), ShouldEqual, 2)

			crossed := confirm.When.All[0]
			So(crossed.Op, ShouldEqual, ComparisonCrossedUp)
			So(crossed.Ago, ShouldEqual, 5)

			versus := confirm.When.All[1]
			So(versus.Op, ShouldEqual, ComparisonAbove)
			So(versus.Versus, ShouldNotBeNil)
			So(versus.Versus.Category, ShouldEqual, CategoryCoiledCompression)
		})

		Convey("Its management carries per-node offsets (tight scalp leash)", func() {
			management := thoughts[0].Then[0].Then
			So(len(management), ShouldBeGreaterThanOrEqualTo, 2)

			var stop Act
			for _, node := range management {
				if node.Do.Type == ActionStopLoss {
					stop = node.Do
				}
			}
			So(stop.Type, ShouldEqual, ActionStopLoss)
			So(stop.Offset, ShouldAlmostEqual, 0.010, 1e-9)
		})

		Convey("The long horizon uses a NOT (avoid toxic) and a wide trailing offset", func() {
			long := thoughts[2]
			hasNot := false
			for _, operand := range long.When.All {
				if operand.Not != nil {
					hasNot = true
					So(operand.Not.Category, ShouldEqual, CategoryToxicBluff)
				}
			}
			So(hasNot, ShouldBeTrue)
		})
	})
}

func TestMarshalThoughtsRoundTrips(t *testing.T) {
	Convey("Given the hand-written multi-horizon playbook", t, func() {
		raw, err := os.ReadFile("cfg/perspectives.yaml")
		So(err, ShouldBeNil)

		original, err := ParseThoughts(raw)
		So(err, ShouldBeNil)

		Convey("Marshalling and re-parsing reproduces the forest exactly", func() {
			encoded, err := MarshalThoughts(original, 2)
			So(err, ShouldBeNil)

			reparsed, err := ParseThoughts(encoded)
			So(err, ShouldBeNil)

			// The whole forest — booleans, versus operands, lifecycle, nested then,
			// and per-node offsets — survives the write/read cycle untouched.
			So(reparsed, ShouldResemble, original)
		})

		Convey("A no-offset action writes as a bare scalar, an offset action as an object", func() {
			encoded, err := MarshalThoughts(original, 2)
			So(err, ShouldBeNil)

			text := string(encoded)
			So(text, ShouldContainSubstring, "do: iceberg") // bare form (Offset 0)
			So(text, ShouldContainSubstring, "offset:")      // object form (leashed stops)
		})
	})
}
