package trader

import (
	"iter"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/market"

	. "github.com/smartystreets/goconvey/convey"
)

type fakeSignal struct {
	roles          []string
	observedRole   string
	observedSymbol string
}

func (signal *fakeSignal) Measure(
	artifact *datura.Artifact,
	_ *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		signal.observedRole = datura.Peek[string](artifact, "channel")
		signal.observedSymbol = datura.Peek[string](artifact, "data", 0, "symbol")

		measurement := datura.Acquire("fake", datura.APPJSON)
		measurement.WithRole("measurement")
		measurement.WithScope("BTC/USD")
		measurement.WithOrigin("fake")
		measurement.SetTimestamp(artifact.Timestamp())
		measurement.MergeOutput("value", 1.0)
		measurement.MergeOutput("confidence", 1.0)
		measurement.MergeOutput("entry_baseline", 0.25)
		measurement.MergeOutput("exit_baseline", 0.25)

		yield(measurement)
	}
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
			measurements, err := signals.Measure(channelTicker, data, at)

			Convey("It should call the signal with an in-memory ingest artifact", func() {
				So(err, ShouldBeNil)
				So(len(measurements), ShouldEqual, 1)
				So(bound.observedRole, ShouldEqual, channelTicker)
				So(bound.observedSymbol, ShouldEqual, "BTC/USD")
				So(signals.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}
