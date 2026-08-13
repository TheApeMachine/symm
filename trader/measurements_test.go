package trader

import (
	"context"
	"sync/atomic"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/types"
)

/*
measurementSignal records whether the measurement scheduler selected a source.
*/
type measurementSignal struct {
	source      types.SourceType
	measurement *types.Measurement
	calls       atomic.Int64
}

func (signal *measurementSignal) Name() string {
	return string(signal.source)
}

func (signal *measurementSignal) Type() types.SourceType {
	return signal.source
}

func (signal *measurementSignal) Measure(*types.Symbol) []*types.Measurement {
	signal.calls.Add(1)

	if signal.measurement == nil {
		return nil
	}

	return []*types.Measurement{signal.measurement}
}

func (signal *measurementSignal) Close() error {
	return nil
}

func TestMeasurementsUpdate(t *testing.T) {
	Convey("Given ticker, trade, and book measurement signals", t, func() {
		signals := map[types.SourceType]*measurementSignal{
			types.SourceCorrelation: {source: types.SourceCorrelation},
			types.SourceCVD:         {source: types.SourceCVD},
			types.SourceDepthFlow:   {source: types.SourceDepthFlow},
			types.SourceExhaustion:  {source: types.SourceExhaustion},
			types.SourceHawkes:      {source: types.SourceHawkes},
			types.SourceLeadLag:     {source: types.SourceLeadLag},
			types.SourceLiquidity:   {source: types.SourceLiquidity},
			types.SourcePumpDump:    {source: types.SourcePumpDump},
			types.SourceSentiment:   {source: types.SourceSentiment},
			types.SourceToxicity:    {source: types.SourceToxicity},
		}
		measurements := &Measurements{
			ctx: context.Background(),
			signals: []types.Signal{
				signals[types.SourceCorrelation],
				signals[types.SourceCVD],
				signals[types.SourceDepthFlow],
				signals[types.SourceExhaustion],
				signals[types.SourceHawkes],
				signals[types.SourceLeadLag],
				signals[types.SourceLiquidity],
				signals[types.SourcePumpDump],
				signals[types.SourceSentiment],
				signals[types.SourceToxicity],
			},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")

		So(signals, ShouldHaveLength, len(types.SignalSources))

		for _, source := range types.SignalSources {
			So(signals[source], ShouldNotBeNil)
		}

		Convey("A ticker update should run only the ticker signals", func() {
			ready, err := measurements.Update(thesis, types.TickerReceivers)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			expected := map[types.SourceType]int64{
				types.SourceCorrelation: 1,
				types.SourceLeadLag:     1,
				types.SourceLiquidity:   1,
				types.SourcePumpDump:    1,
				types.SourceSentiment:   1,
			}

			for source, signal := range signals {
				So(signal.calls.Load(), ShouldEqual, expected[source])
			}
		})

		Convey("A trade update should run only the trade signals", func() {
			ready, err := measurements.Update(thesis, types.TradeReceivers)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			expected := map[types.SourceType]int64{
				types.SourceCVD:        1,
				types.SourceDepthFlow:  1,
				types.SourceExhaustion: 1,
				types.SourceHawkes:     1,
				types.SourcePumpDump:   1,
				types.SourceToxicity:   1,
			}

			for source, signal := range signals {
				So(signal.calls.Load(), ShouldEqual, expected[source])
			}
		})

		Convey("A book update should run only the book signals", func() {
			ready, err := measurements.Update(thesis, types.BookReceivers)
			So(err, ShouldBeNil)
			So(ready, ShouldBeFalse)
			expected := map[types.SourceType]int64{
				types.SourceDepthFlow:  1,
				types.SourceExhaustion: 1,
				types.SourceToxicity:   1,
			}

			for source, signal := range signals {
				So(signal.calls.Load(), ShouldEqual, expected[source])
			}
		})

		Convey("Any normalized source observation should report a queued resonance input", func() {
			value := 0.0
			thesis.Tick = 27

			signals[types.SourceHawkes].measurement = &types.Measurement{
				Source: types.SourceHawkes,
				Symbol: "BTC/USD",
				Tick:   999,
				Metrics: map[string]types.MetricSample{
					"score": {Normalized: &value},
				},
			}

			ready, err := measurements.Update(thesis, types.TradeReceivers)
			So(err, ShouldBeNil)
			So(ready, ShouldBeTrue)
			So(signals[types.SourceHawkes].measurement.Tick, ShouldEqual, thesis.Tick)
			So(thesis.Symbol("BTC/USD").Tick, ShouldEqual, thesis.Tick)

			for row := range thesis.Symbol("BTC/USD").ResonanceMeasurements() {
				So(row.Tick, ShouldEqual, thesis.Tick)
			}
		})
	})
}

func BenchmarkMeasurementsUpdate(b *testing.B) {
	measurements := &Measurements{
		ctx: context.Background(),
		signals: []types.Signal{
			&measurementSignal{source: types.SourceCorrelation},
			&measurementSignal{source: types.SourceCVD},
			&measurementSignal{source: types.SourceDepthFlow},
		},
	}
	thesis := types.NewThesis(b.Context(), nil)
	thesis.Symbol("BTC/USD")
	b.ReportAllocs()

	for b.Loop() {
		if _, err := measurements.Update(thesis, types.TickerReceivers); err != nil {
			b.Fatal(err)
		}
	}
}
