package trader

import (
	"testing"
	"time"

	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeSignal struct {
	roles          []string
	observedRole   string
	observedSymbol string
}

func (signal *fakeSignal) Measure(
	input market.Input,
	_ *market.CrossSection,
) ([]*logic.Measurement, error) {
	signal.observedRole = input.Role
	signal.observedSymbol = input.Ticker[0].Symbol

	measurement := logic.NewMeasurement(logic.SourcePrediction, "BTC/USD", input.At)

	if err := measurement.ApplyClassifier(1, 1, 0.25, 0.25, 1, map[string]float64{
		"1": 1,
	}); err != nil {
		return nil, err
	}

	return []*logic.Measurement{measurement}, nil
}

func (signal *fakeSignal) IngestRoles() []string {
	return signal.roles
}

func (signal *fakeSignal) Close() error {
	return nil
}

func TestSignalsMeasure(t *testing.T) {
	Convey("Given direct signal bindings", t, func() {
		crossSection, err := market.NewCrossSection()
		So(err, ShouldBeNil)

		bound := &fakeSignal{roles: []string{channelTicker}}
		signals := &Signals{
			crossSection: crossSection,
		}
		signals.Bind(bound)

		at := time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC)
		data := []byte(`[{
			"symbol": "BTC/USD",
			"bid": 99,
			"bid_qty": 2,
			"ask": 101,
			"ask_qty": 1,
			"last": 100,
			"volume": 10,
			"change_pct": 1,
			"timestamp": "2026-07-04T12:00:00Z"
		}]`)

		Convey("When ticker data is measured", func() {
			measurements, snapshots, err := signals.Measure(channelTicker, data, at)

			Convey("It should call the signal with typed market input", func() {
				So(err, ShouldBeNil)
				So(len(measurements), ShouldEqual, 1)
				So(snapshots, ShouldHaveLength, 0)
				So(measurements[0].Distribution[logic.CategoryForecastEdge], ShouldEqual, 1.0)
				So(bound.observedRole, ShouldEqual, channelTicker)
				So(bound.observedSymbol, ShouldEqual, "BTC/USD")
				So(signals.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}
