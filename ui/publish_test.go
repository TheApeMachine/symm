package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func testGaugeArtifact(
	source logic.SourceType,
	scope string,
	category logic.CategoryType,
	confidence float64,
	strength float64,
) *datura.Artifact {
	artifact := datura.Acquire("test", datura.Artifact_Type_json)
	artifact.WithRole("measurement")
	artifact.WithScope(scope)
	_ = artifact.SetOrigin(string(source))
	artifact.Poke(logic.CategoryIndex(category), "classifier", "category")
	artifact.Poke(confidence, "classifier", "confidence")
	artifact.Poke(strength, "classifier", "strength")
	artifact.Poke(2.4, "surprise")
	artifact.Poke(30.0, "elapsed")
	artifact.Poke("2024-01-01T00:00:00Z", "observed_at")

	return artifact
}

func TestGaugeReadingFromArtifact(testingTB *testing.T) {
	Convey("Given a fluid measurement artifact", testingTB, func() {
		artifact := testGaugeArtifact(
			logic.SourceFluid,
			"BTC/USD",
			logic.CategoryLaminar,
			0.71,
			0.36,
		)

		reading := GaugeReadingFromArtifact(artifact)

		Convey("It should expose dashboard gauge wire fields", func() {
			So(reading, ShouldNotBeNil)
			So(reading["source"], ShouldEqual, "fluid")
			So(reading["symbol"], ShouldEqual, "BTC/USD")
			So(reading["confidence"], ShouldEqual, 0.71)
			So(reading["strength"], ShouldEqual, 0.36)
			So(reading["category"], ShouldEqual, "laminar")
			So(reading["calibrated"], ShouldBeTrue)
		})
	})
}

func TestStateFrame(testingTB *testing.T) {
	Convey("Given spectrum measurement artifacts", testingTB, func() {
		measurements := []*datura.Artifact{
			testGaugeArtifact(logic.SourceFluid, "BTC/USD", logic.CategoryLaminar, 0.5, 1.0),
			testGaugeArtifact(logic.SourceCVD, "BTC/USD", logic.CategoryOrganic, 0.6, 1.0),
		}

		frame := StateFrame(measurements, 3, nil)

		Convey("It should publish state frames for the dashboard websocket", func() {
			So(frame["type"], ShouldEqual, "state")
			So(frame["story_ticks"], ShouldEqual, 3)

			gaugeReadings, ok := frame["gauge_readings"].([]map[string]any)

			So(ok, ShouldBeTrue)
			So(len(gaugeReadings), ShouldEqual, 2)
		})
	})
}

func BenchmarkGaugeReadingFromArtifact(b *testing.B) {
	artifact := testGaugeArtifact(
		logic.SourceFluid,
		"BTC/USD",
		logic.CategoryLaminar,
		0.71,
		0.36,
	)

	b.ReportAllocs()

	for b.Loop() {
		_ = GaugeReadingFromArtifact(artifact)
	}
}
