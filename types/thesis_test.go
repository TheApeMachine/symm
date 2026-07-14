package types

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestThesisPublish(t *testing.T) {
	Convey("Given a thesis carrying signal measurements and logic results", t, func() {
		uiHub := make(chan []byte, 1)
		thesis := NewThesis(uiHub)

		thesis.Measurements = append(thesis.Measurements, &Measurement{
			Source: SourcePumpDump, Metric: MetricStrength, Symbol: "BTC/USD", At: time.Unix(1, 0), Raw: 0.7,
		})
		thesis.Graphs["BTC/USD"] = NewGraph("BTC/USD")
		thesis.Forecasts = append(thesis.Forecasts, Forecasts{
			Source: "manifold", Symbol: "BTC/USD", At: time.Unix(1, 0),
		})
		thesis.Hypotheses = append(thesis.Hypotheses, Hypothesis{
			Source: SourceCausal, Symbol: "BTC/USD", At: time.Unix(1, 0),
		})

		Convey("When published", func() {
			thesis.Publish()

			Convey("Then only the measurement slice reaches the wire", func() {
				payload := <-uiHub

				var frame struct {
					Measurements []Measurement `json:"measurements"`
				}

				So(json.Unmarshal(payload, &frame), ShouldBeNil)
				So(len(frame.Measurements), ShouldEqual, 1)
				So(frame.Measurements[0].Symbol, ShouldEqual, "BTC/USD")
				So(frame.Measurements[0].Source, ShouldEqual, SourcePumpDump)
			})
		})
	})
}
