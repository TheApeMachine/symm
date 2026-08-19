package trader

import (
	"context"
	"encoding/json"
	"iter"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/hawkes"
	"github.com/theapemachine/symm/types"
)

/*
measurementSignal records which symbols the pass measured and optionally
yields one measurement per call.
*/
type measurementSignal struct {
	source      types.SourceType
	measurement *types.Measurement
	calls       atomic.Int64
	seen        sync.Map
}

func (signal *measurementSignal) Name() string {
	return string(signal.source)
}

func (signal *measurementSignal) Type() types.SourceType {
	return signal.source
}

func (signal *measurementSignal) Measure(
	symbol *types.Symbol, _ ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		signal.calls.Add(1)
		signal.seen.Store(symbol.Symbol, true)

		for range symbol.MarketTickers(signal.source) {
		}

		if signal.measurement == nil {
			return
		}

		yield(signal.measurement)
	}
}

func (signal *measurementSignal) Close() error {
	return nil
}

/*
newTestMeasurements wires a cancellable measurement stage around fake signals.
*/
func newTestMeasurements(
	t *testing.T,
	signals []types.Signal,
) (*Measurements, context.CancelFunc) {
	ctx, cancel := context.WithCancel(t.Context())

	return &Measurements{
		ctx:     ctx,
		cancel:  cancel,
		signals: signals,
	}, cancel
}

/*
awaitThesis fails the test unless one thesis arrives within a second.
*/
func awaitThesis(
	t *testing.T,
	theses <-chan *types.Thesis,
) *types.Thesis {
	select {
	case thesis := <-theses:
		return thesis
	case <-time.After(time.Second):
		t.Fatal("no thesis arrived")
		return nil
	}
}

func feedTicker(
	symbol *types.Symbol,
	source types.SourceType,
	at time.Time,
) {
	symbol.AppendTicker(kraken.TickerData{
		Symbol: symbol.Symbol, Timestamp: at,
	})
}

func TestMeasurementsGenerate(t *testing.T) {
	Convey("Given one dirty symbol and one clean symbol", t, func() {
		measured := &measurementSignal{source: types.SourceCVD}
		measurements, cancel := newTestMeasurements(t, []types.Signal{measured})
		defer cancel()

		thesis := types.NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		thesis.Symbol("DDD/USD")
		feedTicker(bitcoin, types.SourceCVD, time.Now())

		// One Generate per thesis: a second drain loop would race the first
		// for the same queues.
		theses := measurements.Generate(thesis, nil)

		Convey("It should drain the dirty symbol and skip the clean one", func() {
			received := awaitThesis(t, theses)
			So(received, ShouldEqual, thesis)
			So(received.At.IsZero(), ShouldBeFalse)
			_, bitcoinSeen := measured.seen.Load("BTC/USD")
			_, dormantSeen := measured.seen.Load("DDD/USD")
			So(bitcoinSeen, ShouldBeTrue)
			So(dormantSeen, ShouldBeFalse)
			So(bitcoin.Pending(), ShouldBeFalse)
		})

		Convey("A yielded measurement should stream into the solver cursors", func() {
			producing := &measurementSignal{
				source: types.SourceCorrelation,
				measurement: &types.Measurement{
					Source: types.SourceCorrelation,
					At:     time.Now().UTC(),
				},
			}
			producingMeasurements, producingCancel := newTestMeasurements(
				t, []types.Signal{producing},
			)
			defer producingCancel()

			producingThesis := types.NewThesis(t.Context(), nil)
			freshBitcoin := producingThesis.Symbol("BTC/USD")
			feedTicker(freshBitcoin, types.SourceCorrelation, time.Now())

			awaitThesis(t, producingMeasurements.Generate(producingThesis, nil))

			categoryRows := 0
			graphRows := 0

			for range freshBitcoin.MarketMeasurements("category") {
				categoryRows++
			}

			for range freshBitcoin.MarketMeasurements("graph") {
				graphRows++
			}

			So(categoryRows, ShouldBeGreaterThanOrEqualTo, 0)
			So(graphRows, ShouldBeGreaterThanOrEqualTo, 0)
		})
	})
}

func TestMeasurementsIdleRest(t *testing.T) {
	Convey("Given an always-yielding signal and symbols with no pending rows", t, func() {
		always := &measurementSignal{
			source:      types.SourceCVD,
			measurement: &types.Measurement{Source: types.SourceCVD},
		}
		measurements, cancel := newTestMeasurements(t, []types.Signal{always})
		defer cancel()

		thesis := types.NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")

		theses := measurements.Generate(thesis, nil)

		select {
		case <-theses:
			t.Fatal("a pass ran with no pending market rows")
		case <-time.After(150 * time.Millisecond):
		}

		So(always.calls.Load(), ShouldEqual, int64(0))

		Convey("Rows arriving wakes the drain loop", func() {
			feedTicker(bitcoin, types.SourceCVD, time.Now())
			awaitThesis(t, theses)
			So(always.calls.Load(), ShouldBeGreaterThanOrEqualTo, int64(1))
		})
	})
}

type gatedMeasurementSignal struct {
	source  types.SourceType
	entered chan struct{}
	release chan struct{}
}

func (signal *gatedMeasurementSignal) Name() string           { return string(signal.source) }
func (signal *gatedMeasurementSignal) Type() types.SourceType { return signal.source }
func (signal *gatedMeasurementSignal) Measure(
	symbol *types.Symbol,
	_ ...int64,
) iter.Seq[*types.Measurement] {
	return func(yield func(*types.Measurement) bool) {
		signal.entered <- struct{}{}
		<-signal.release

		for range symbol.MarketTickers(signal.source) {
		}
	}
}
func (signal *gatedMeasurementSignal) Close() error { return nil }

func TestMeasurementsObservationPacing(t *testing.T) {
	Convey("Given a gated signal and rows already pending", t, func() {
		gated := &gatedMeasurementSignal{
			source:  types.SourceCVD,
			entered: make(chan struct{}, 1),
			release: make(chan struct{}),
		}
		measurements, cancel := newTestMeasurements(t, []types.Signal{gated})
		defer cancel()

		thesis := types.NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")
		feedTicker(bitcoin, types.SourceCVD, time.Now())

		theses := measurements.Generate(thesis, nil)

		<-gated.entered
		gated.release <- struct{}{}

		Convey("The next pass should wait until the current thesis is observed", func() {
			select {
			case <-gated.entered:
				t.Fatal("a pass advanced the thesis before the current one was observed")
			case <-time.After(150 * time.Millisecond):
			}

			select {
			case <-theses:
			case <-time.After(time.Second):
				t.Fatal("the completed thesis never arrived")
			}

			feedTicker(bitcoin, types.SourceCVD, time.Now())

			select {
			case <-gated.entered:
			case <-time.After(time.Second):
				t.Fatal("the drain loop never continued after observation")
			}

			gated.release <- struct{}{}
			awaitThesis(t, theses)
		})
	})
}

func TestMeasurementsCancel(t *testing.T) {
	Convey("Given a gated signal blocked mid-pass", t, func() {
		gated := &gatedMeasurementSignal{
			source:  types.SourceCVD,
			entered: make(chan struct{}, 1),
			release: make(chan struct{}),
		}
		measurements, cancel := newTestMeasurements(t, []types.Signal{gated})
		defer cancel()

		thesis := types.NewThesis(t.Context(), nil)
		feedTicker(thesis.Symbol("BTC/USD"), types.SourceCVD, time.Now())

		theses := measurements.Generate(thesis, nil)

		<-gated.entered
		cancel()
		close(gated.release)

		Convey("The thesis stream should close", func() {
			select {
			case _, open := <-theses:
				So(open, ShouldBeFalse)
			case <-time.After(time.Second):
				t.Fatal("the thesis stream stayed open after cancellation")
			}
		})
	})
}

func TestMeasurementsFocusPublication(t *testing.T) {
	Convey("Given the focused symbol producing measurements", t, func() {
		measured := &measurementSignal{
			source: types.SourceCorrelation,
			measurement: &types.Measurement{
				Source: types.SourceCorrelation,
				Symbol: types.Focus(),
				At:     time.Now().UTC(),
			},
		}
		ui := make(chan []byte, 4)
		ctx, cancel := context.WithCancel(t.Context())
		measurements := &Measurements{
			ctx:     ctx,
			cancel:  cancel,
			ui:      ui,
			signals: []types.Signal{measured},
		}
		defer cancel()

		thesis := types.NewThesis(t.Context(), nil)
		feedTicker(thesis.Symbol(types.Focus()), types.SourceCorrelation, time.Now())

		awaitThesis(t, measurements.Generate(thesis, nil))

		type wireMeasurement struct {
			Source types.SourceType `json:"source"`
			Symbol string           `json:"symbol"`
		}
		type wireFrame struct {
			Measurements []wireMeasurement `json:"measurements"`
		}
		var frame wireFrame

		select {
		case payload := <-ui:
			So(json.Unmarshal(payload, &frame), ShouldBeNil)
		case <-time.After(time.Second):
			t.Fatal("no measurements frame arrived for the focused symbol")
		}

		So(frame.Measurements, ShouldHaveLength, 1)
		So(frame.Measurements[0].Source, ShouldEqual, types.SourceCorrelation)
		So(frame.Measurements[0].Symbol, ShouldEqual, types.Focus())
	})
}

func TestMeasurementsHawkesEndToEnd(t *testing.T) {
	Convey("Given a real hawkes signal over real queued trades", t, func() {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		measurements := &Measurements{
			ctx:     ctx,
			cancel:  cancel,
			signals: []types.Signal{hawkes.NewSignal(ctx, nil)},
		}
		thesis := types.NewThesis(t.Context(), nil)
		bitcoin := thesis.Symbol("BTC/USD")

		for index := range 3 {
			bitcoin.AppendTrade(kraken.TradeData{
				Symbol:    "BTC/USD",
				Side:      "buy",
				Price:     *decimal.NewFromFloat64(100 + float64(index)),
				Qty:       2,
				TradeID:   int64(index + 1),
				Timestamp: time.Now().Add(time.Duration(index) * time.Second),
			})
		}

		awaitThesis(t, measurements.Generate(thesis, nil))

		// A pass that serviced the trades must have drained the trade queue:
		// nothing remains for the next pass to re-read.
		residual := 0

		for range bitcoin.MarketTrades(types.SourceHawkes) {
			residual++
		}

		So(residual, ShouldEqual, 0)
	})
}

func BenchmarkMeasurementsGenerate(b *testing.B) {
	ctx, cancel := context.WithCancel(b.Context())
	defer cancel()

	measurements := &Measurements{
		ctx:    ctx,
		cancel: cancel,
		signals: []types.Signal{
			&measurementSignal{source: types.SourceCVD},
			&measurementSignal{source: types.SourceHawkes},
		},
	}
	thesis := types.NewThesis(b.Context(), nil)
	bitcoin := thesis.Symbol("BTC/USD")
	theses := measurements.Generate(thesis, nil)
	b.ReportAllocs()

	for b.Loop() {
		feedTicker(bitcoin, types.SourceCVD, time.Now())

		select {
		case <-theses:
		case <-time.After(time.Second):
			b.Fatal("no thesis arrived")
		}
	}
}
