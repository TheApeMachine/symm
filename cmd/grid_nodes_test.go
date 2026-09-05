package cmd

import (
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/signal/derivatives"
	"github.com/theapemachine/symm/strategy"
	"reflect"
	"strconv"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/logic/cognition"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/signal/depthflow"
	"github.com/theapemachine/symm/signal/morphology"
	markettest "github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/types"
)

func TestGridNodeStep(t *testing.T) {
	Convey("Given derivative facts mapped by the transport onto an executable spot instrument", t, func() {
		signal := derivatives.NewSignal(t.Context())
		grid := learning.NewGrid()
		learner, err := strategy.NewAgent(t.Context(), grid, &gridBoundaryBook{}, func(string) kraken.InstrumentPair { return kraken.InstrumentPair{Symbol: "TEST/USD"} }, func(string) *kraken.TradeVolumeFee { return nil }, decimal.NewFromInt64(200), func(hindsight.LearningEvent) error { return nil })
		So(err, ShouldBeNil)
		node := &gridNode{Grid: grid, learner: learner, prepare: []runtime.Node[*types.Envelope]{signal}}
		Reset(func() { So(signal.Close(), ShouldBeNil) })
		for index, price := range []int64{100, 103, 101, 105, 98} {
			envelope := types.NewEnvelope(types.EnvelopeFuturesTicker)
			envelope.FuturesTickerData = kraken.FuturesTickerData{Symbol: "TEST/USD", Timestamp: time.Unix(100+int64(index), 0), Last: decimal.NewFromInt64(price), IndexPrice: decimal.NewFromInt64(100), MarkPrice: decimal.NewFromInt64(100), OpenInterest: float64(20 + index)}
			So(node.Step(envelope), ShouldEqual, envelope)
			So(node.Error(), ShouldBeNil)
			So(envelope.Derivatives, ShouldNotBeNil)
		}
		So(grid.Rows, ShouldResemble, []string{"TEST/USD"})
		So(grid.Column("derivatives", "basis"), ShouldBeGreaterThanOrEqualTo, 0)
	})
	Convey("Given cognition and numerical learning sharing one event owner", t, func() {
		node := &gridNode{Grid: learning.NewGrid(), cognition: cognition.NewSolver(t.Context())}
		for index, regime := range []types.CategoryType{
			types.OrganicTrend, types.Turbulent, types.OrganicTrend, types.Exhaustion,
		} {
			envelope := types.NewEnvelope(types.EnvelopeTicker)
			envelope.TickerData.Symbol = "TEST/USD"
			envelope.Categories = []types.Category{{At: time.Unix(int64(index+1), 0),
				Symbol: "TEST/USD", Type: regime, Confidence: 1, Strength: 1, Maturity: 1}}
			So(node.Step(envelope), ShouldEqual, envelope)
			So(node.Error(), ShouldBeNil)
			So(envelope.Cognition, ShouldNotBeNil)
			So(node.Version, ShouldEqual, index+1)
			So(node.projections["TEST/USD"][0].Metrics["Confidence"].Raw,
				ShouldEqual, envelope.Cognition.Confidence)
		}
	})

	Convey("Given every canonical signal slot", t, func() {
		node := &gridNode{Grid: learning.NewGrid()}
		envelope := types.NewEnvelope(types.EnvelopeTicker)
		fields := []**data.Measurement[float64]{
			&envelope.Correlation, &envelope.LeadLag, &envelope.Liquidity,
			&envelope.Sentiment, &envelope.CVD, &envelope.DepthFlow,
			&envelope.Morphology, &envelope.Hawkes, &envelope.PumpDump,
			&envelope.Toxicity, &envelope.Derivatives,
		}

		for index, field := range fields {
			measurement := data.NewMeasurement[float64](
				"", "TEST/USD", strconv.Itoa(index), time.Time{}, time.Time{},
			)
			measurement.PutMetric(data.Metric[float64]{Label: "value", Raw: float64(index)})
			*field = measurement
		}

		So(node.Step(envelope), ShouldEqual, envelope)
		So(node.Error(), ShouldBeNil)
		So(len(node.Columns), ShouldEqual, len(fields))
		So(node.Version, ShouldEqual, 1)
		So(node.Rows, ShouldResemble, []string{"TEST/USD"})

		// A newly added signal field must not silently miss the grid stage.
		measurementType := reflect.TypeOf((*data.Measurement[float64])(nil))
		envelopeType := reflect.TypeOf(types.Envelope{})
		slots := 0

		for index := range envelopeType.NumField() {
			if envelopeType.Field(index).Type == measurementType {
				slots++
			}
		}

		So(len(node.Columns), ShouldEqual, slots)

		for index, field := range fields {
			So((*field).Metrics["value"].Coordinates, ShouldNotBeNil)
			So(node.Values[0][index], ShouldEqual, float64(index))
		}
	})

	Convey("Given actual signal projections across a multi-leg Level3 tape", t, func() {
		depth := depthflow.NewSignal(t.Context())
		shape := morphology.NewSignal(t.Context())
		sink := runtime.NewSink[*types.Envelope](1)
		node := &gridNode{Grid: learning.NewGrid(),
			prepare: []runtime.Node[*types.Envelope]{depth, shape},
			publish: []runtime.Node[*types.Envelope]{sink},
		}
		Reset(func() {
			So(depth.Close(), ShouldBeNil)
			So(shape.Close(), ShouldBeNil)
		})
		tape := markettest.NewLevel3Tape("TEST/USD", time.Unix(1, 0))

		for _, message := range tape.Messages {
			envelope := types.NewEnvelope(types.EnvelopeLevel3)
			envelope.Level3Data = message
			So(node.Step(envelope), ShouldEqual, envelope)
			So(node.Error(), ShouldBeNil)
			So(<-sink.Out(), ShouldEqual, envelope)

			for _, measurement := range envelope.SignalMeasurements() {
				if measurement == nil {
					continue
				}

				for _, metric := range measurement.Metrics {
					So(metric.Coordinates, ShouldNotBeNil)
				}
			}
		}

		So(node.Version, ShouldEqual, len(tape.Messages))
		So(node.Columns, ShouldNotBeEmpty)
		So(node.Rows, ShouldResemble, []string{"TEST/USD"})
	})
}

func BenchmarkGridNodeStep(b *testing.B) {
	depth := depthflow.NewSignal(b.Context())
	shape := morphology.NewSignal(b.Context())
	sink := runtime.NewSink[*types.Envelope](1)
	node := &gridNode{Grid: learning.NewGrid(),
		prepare: []runtime.Node[*types.Envelope]{depth, shape},
		publish: []runtime.Node[*types.Envelope]{sink},
	}
	b.Cleanup(func() {
		if err := depth.Close(); err != nil {
			b.Error(err)
		}

		if err := shape.Close(); err != nil {
			b.Error(err)
		}
	})
	tape := markettest.NewLevel3Tape("TEST/USD", time.Unix(1, 0))
	envelopes := make([]*types.Envelope, 0, len(tape.Messages))

	for _, message := range tape.Messages {
		envelope := types.NewEnvelope(types.EnvelopeLevel3)
		envelope.Level3Data = message
		node.Step(envelope)

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}

		<-sink.Out()
		envelopes = append(envelopes, envelope)
	}

	index := 0
	// Advance the fixture's own clock each cycle, so producer baselines see
	// fresh observations rather than repeatedly processing old timestamps.
	cycle := tape.Messages[len(tape.Messages)-1].Timestamp.Sub(tape.Messages[0].Timestamp) +
		tape.Messages[1].Timestamp.Sub(tape.Messages[0].Timestamp)
	b.ReportAllocs()

	for b.Loop() {
		envelopes[index].Level3Data.Timestamp = envelopes[index].Level3Data.Timestamp.Add(cycle)
		node.Step(envelopes[index])

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}

		<-sink.Out()
		index = (index + 1) % len(envelopes)
	}
}

func BenchmarkGridNodeStepCognition(b *testing.B) {
	node := &gridNode{Grid: learning.NewGrid(), cognition: cognition.NewSolver(b.Context())}
	envelope := types.NewEnvelope(types.EnvelopeTicker)
	envelope.TickerData.Symbol = "TEST/USD"
	envelope.Categories = []types.Category{{
		At: time.Unix(1, 0), Symbol: "TEST/USD", Confidence: 1, Strength: 1, Maturity: 1,
	}}
	// Repeated regime transitions exercise the real growing episodic model,
	// including inference, scalar projection and grid organization.
	regimes := [...]types.CategoryType{types.OrganicTrend, types.Turbulent, types.Exhaustion}
	index := 0
	b.ReportAllocs()

	for b.Loop() {
		envelope.Categories[0].Type = regimes[index%len(regimes)]
		envelope.Categories[0].At = time.Unix(int64(index+1), 0)
		node.Step(envelope)

		if err := node.Error(); err != nil {
			b.Fatal(err)
		}

		index++
	}
}

/* gridBoundaryBook fails loudly if information-only envelopes request execution depth. */
type gridBoundaryBook struct{}

func (*gridBoundaryBook) Book(string, func(*spotbook.Book)) {
	panic("futures cannot enter the executable spot loop")
}
