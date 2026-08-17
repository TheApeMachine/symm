package trader

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func (signal *measurementSignal) Measure(
	_ *types.Symbol, ticks ...int64,
) []*types.Measurement {
	signal.calls.Add(1)

	if signal.measurement == nil {
		return nil
	}

	if len(ticks) > 0 {
		signal.measurement.Tick = ticks[0]
	}

	return []*types.Measurement{signal.measurement}
}

func (signal *measurementSignal) Close() error {
	return nil
}

func TestMeasurementsGenerate(t *testing.T) {
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
			err := measurements.Generate(thesis, types.TickerReceivers)
			So(err, ShouldBeNil)
			expected := map[types.SourceType]int64{
				types.SourceCorrelation: 1,
				types.SourceCVD:         1,
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
			err := measurements.Generate(thesis, types.TradeReceivers)
			So(err, ShouldBeNil)
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
			err := measurements.Generate(thesis, types.BookReceivers)
			So(err, ShouldBeNil)
			expected := map[types.SourceType]int64{
				types.SourceDepthFlow:  1,
				types.SourceExhaustion: 1,
			}

			for source, signal := range signals {
				So(signal.calls.Load(), ShouldEqual, expected[source])
			}
		})

		Convey("It should publish the engine tick count for the header", func() {
			ui := make(chan []byte, 32)
			measurements.ui = ui
			thesis.Tick = 41

			err := measurements.Generate(thesis, types.TickerReceivers)
			So(err, ShouldBeNil)
			So(thesis.Tick, ShouldEqual, 42)

			var frame struct {
				Tick struct {
					Count int64 `json:"count"`
				} `json:"tick"`
			}

			for range len(ui) {
				payload := <-ui

				if json.Unmarshal(payload, &frame) != nil {
					continue
				}

				if frame.Tick.Count == 0 {
					continue
				}

				break
			}

			So(frame.Tick.Count, ShouldEqual, 42)
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

			err := measurements.Generate(thesis, types.TradeReceivers)
			So(err, ShouldBeNil)
			So(signals[types.SourceHawkes].measurement.Tick, ShouldEqual, thesis.Tick)
			So(thesis.Symbol("BTC/USD").Tick, ShouldEqual, thesis.Tick)

			for row := range thesis.Symbol("BTC/USD").ResonanceMeasurements() {
				So(row.Tick, ShouldEqual, thesis.Tick)
			}
		})
	})
}

func TestMeasurementsUIPublication(t *testing.T) {
	Convey("Given two selected signals that both produce a measurement", t, func() {
		correlation := &measurementSignal{
			source: types.SourceCorrelation,
			measurement: &types.Measurement{
				Source: types.SourceCorrelation,
				Symbol: "BTC/USD",
			},
		}
		cvd := &measurementSignal{
			source: types.SourceCVD,
			measurement: &types.Measurement{
				Source: types.SourceCVD,
				Symbol: "BTC/USD",
			},
		}
		ui := make(chan []byte, 4)
		measurements := &Measurements{
			ctx:     context.Background(),
			ui:      ui,
			signals: []types.Signal{correlation, cvd},
		}
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("BTC/USD")

		err := measurements.Generate(thesis, []types.SourceType{
			types.SourceCorrelation,
			types.SourceCVD,
		})
		So(err, ShouldBeNil)

		type wireMeasurement struct {
			Source types.SourceType `json:"source"`
		}
		type wireFrame struct {
			Tick struct {
				Count int64 `json:"count"`
			} `json:"tick"`
			Activity     map[string]string `json:"activity"`
			Measurements []wireMeasurement `json:"measurements"`
		}
		frames := make([]wireFrame, 0, len(ui))

		for len(ui) > 0 {
			var frame wireFrame
			So(json.Unmarshal(<-ui, &frame), ShouldBeNil)
			frames = append(frames, frame)
		}

		Convey("It should send one cut-start frame and one batched completion frame", func() {
			So(frames, ShouldHaveLength, 2)
			So(frames[0].Tick.Count, ShouldEqual, thesis.Tick)
			So(frames[0].Activity, ShouldResemble, map[string]string{
				string(types.SourceCorrelation): "running",
				string(types.SourceCVD):         "running",
			})
			So(frames[0].Measurements, ShouldBeEmpty)
			So(frames[1].Activity, ShouldResemble, map[string]string{
				string(types.SourceCorrelation): "done",
				string(types.SourceCVD):         "done",
			})
			So(frames[1].Measurements, ShouldResemble, []wireMeasurement{
				{Source: types.SourceCorrelation},
				{Source: types.SourceCVD},
			})
		})
	})
}

func BenchmarkMeasurementsGenerate(b *testing.B) {
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
		if err := measurements.Generate(thesis, types.TickerReceivers); err != nil {
			b.Fatal(err)
		}
	}
}

type cohortMeasurementSignal struct {
	source  types.SourceType
	calls   atomic.Int64
	mu      sync.Mutex
	symbols []string
}

func (signal *cohortMeasurementSignal) Name() string           { return string(signal.source) }
func (signal *cohortMeasurementSignal) Type() types.SourceType { return signal.source }
func (signal *cohortMeasurementSignal) Measure(*types.Symbol, ...int64) []*types.Measurement {
	panic("cohort signal must be measured once for the complete thesis set")
}
func (signal *cohortMeasurementSignal) MeasureCohort(
	symbols []*types.Symbol,
	_ ...int64,
) []*types.Measurement {
	signal.calls.Add(1)
	signal.mu.Lock()
	defer signal.mu.Unlock()
	signal.symbols = signal.symbols[:0]

	for _, symbol := range symbols {
		signal.symbols = append(signal.symbols, symbol.Symbol)
	}

	return nil
}
func (signal *cohortMeasurementSignal) Close() error { return nil }

type parallelMeasurementSignal struct {
	source  types.SourceType
	entered chan string
	release <-chan struct{}
}

func (signal *parallelMeasurementSignal) Name() string           { return string(signal.source) }
func (signal *parallelMeasurementSignal) Type() types.SourceType { return signal.source }
func (signal *parallelMeasurementSignal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) []*types.Measurement {
	signal.entered <- symbol.Symbol
	<-signal.release
	return nil
}
func (signal *parallelMeasurementSignal) Close() error { return nil }

func TestMeasurementsStreamingOwnership(t *testing.T) {
	Convey("Given three symbols in one thesis universe", t, func() {
		thesis := types.NewThesis(t.Context(), nil)
		thesis.Symbol("AAA/USD")
		thesis.Symbol("BBB/USD")
		thesis.Symbol("CCC/USD")
		cohort := &cohortMeasurementSignal{source: types.SourceCorrelation}
		entered := make(chan string, 3)
		release := make(chan struct{})
		local := &parallelMeasurementSignal{
			source: types.SourceCVD, entered: entered, release: release,
		}
		measurements := &Measurements{
			ctx:     context.Background(),
			signals: []types.Signal{cohort, local},
		}
		done := make(chan error, 1)

		go func() {
			err := measurements.Generate(
				thesis,
				[]types.SourceType{types.SourceCorrelation, types.SourceCVD},
			)
			done <- err
		}()

		observed := make(map[string]bool)

		for len(observed) < 3 {
			select {
			case symbol := <-entered:
				observed[symbol] = true
			case <-time.After(time.Second):
				t.Fatal("independent symbol workers did not enter concurrently")
			}
		}

		close(release)

		select {
		case err := <-done:
			So(err, ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("measurement generation did not complete")
		}

		cohort.mu.Lock()
		cohortSymbols := append([]string(nil), cohort.symbols...)
		cohort.mu.Unlock()

		Convey("It should run one complete cohort and one transient worker per symbol", func() {
			So(cohort.calls.Load(), ShouldEqual, int64(1))
			So(cohortSymbols, ShouldResemble, []string{"AAA/USD", "BBB/USD", "CCC/USD"})
			So(observed, ShouldResemble, map[string]bool{
				"AAA/USD": true,
				"BBB/USD": true,
				"CCC/USD": true,
			})
			So(thesis.Symbol("AAA/USD").Tick, ShouldEqual, thesis.Tick)
			So(thesis.Symbol("BBB/USD").Tick, ShouldEqual, thesis.Tick)
			So(thesis.Symbol("CCC/USD").Tick, ShouldEqual, thesis.Tick)
		})
	})
}
