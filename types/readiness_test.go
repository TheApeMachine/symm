package types

import (
	"encoding/json"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestReadinessStamp(t *testing.T) {
	Convey("Given one symbol readiness publisher", t, func() {
		updates := make(chan []byte, 1)
		readiness := NewReadiness("BTC/USD", updates)

		readiness.Stamp(SourceCVD)

		var frame struct {
			Readiness []Readiness `json:"readiness"`
		}
		So(json.Unmarshal(<-updates, &frame), ShouldBeNil)

		Convey("Then it should identify the symbol whose gate changed", func() {
			So(frame.Readiness, ShouldHaveLength, 1)
			So(frame.Readiness[0].Symbol, ShouldEqual, "BTC/USD")
			So(frame.Readiness[0].CVD, ShouldBeTrue)
		})
	})
}

func TestReadinessReset(t *testing.T) {
	Convey("Given published readiness for one symbol", t, func() {
		updates := make(chan []byte, 2)
		readiness := NewReadiness("BTC/USD", updates)
		readiness.Stamp(SourceCVD)
		<-updates

		readiness.Reset()

		var frame struct {
			Readiness []Readiness `json:"readiness"`
		}
		So(json.Unmarshal(<-updates, &frame), ShouldBeNil)

		Convey("Then it should publish that the symbol gate cleared", func() {
			So(frame.Readiness, ShouldHaveLength, 1)
			So(frame.Readiness[0].Symbol, ShouldEqual, "BTC/USD")
			So(frame.Readiness[0].CVD, ShouldBeFalse)
		})
	})
}

func TestReadinessComplete(t *testing.T) {
	Convey("Given every required stage has stamped one symbol", t, func() {
		readiness := NewReadiness("BTC/USD", nil)

		for _, source := range []SourceType{
			SourceCorrelation,
			SourceCVD,
			SourceDepthFlow,
			SourceExhaustion,
			SourceHawkes,
			SourceLeadLag,
			SourceLiquidity,
			SourcePumpDump,
			SourceSentiment,
			SourceToxicity,
			SourceCategory,
			SourceCognition,
			SourceManifold,
			SourceResonance,
			SourceCausal,
			SourceGraph,
			SourcePlanner,
		} {
			readiness.Stamp(source)
		}

		Convey("Then the complete check should return true", func() {
			So(readiness.Complete(), ShouldBeTrue)
		})
	})
}
