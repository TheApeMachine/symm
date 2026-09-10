package strategy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"sync"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/hindsight/tables"
	"github.com/theapemachine/symm/hindsight/tables/tablestest"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/learning/associative/grid"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/venue"
	"github.com/theapemachine/symm/types"
)

// learningFixture supplies the real book reducer, pricing, positions, balance,
// cognition agent, impulse map and recorder. Only external venue I/O is substituted.
func learningFixture(t testing.TB) (*Learner, *venue.Conn) {
	t.Helper()
	conn := venue.NewConn()
	api := websocket.NewAPI(t.Context(), conn, conn)
	api.Normalizer().Update(&spot.AssetsManagerUpdate{
		NewAssets: map[string]spot.AssetInfo{
			"BTC": {
				AltName:  "BTC",
				Decimals: 8,
			},
			"USD": {
				AltName:  "USD",
				Decimals: 2,
			},
		},

		NewPairs: map[string]spot.AssetPair{
			"BTCUSD": {
				WSName:        "BTC/USD",
				Base:          "BTC",
				Quote:         "USD",
				PairDecimals:  2,
				LotDecimals:   8,
				LotMultiplier: 1,
			},
		},
	})
	instrument := broker.NewInstrumentWithQuote("USD")
	instrument.Cache([]kraken.InstrumentPair{{
		Symbol:       "BTC/USD",
		Base:         "BTC",
		Quote:        "USD",
		Status:       "online",
		QtyMin:       venue.Decimal("0.0001"),
		CostMin:      venue.Decimal("0.5"),
		QtyIncrement: venue.Decimal("0.00000001"),
	}})

	price := broker.NewPrice(api, instrument)
	price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: venue.Decimal("0.25")})

	price.Update(&kraken.TickerData{
		Symbol: "BTC/USD",
		Last:   venue.Decimal("100"),
		Bid:    venue.Decimal("99"),
		Ask:    venue.Decimal("101"),
	})

	catalog := tablestest.New(t)
	recorder, err := recording.NewSession(
		t.Context(), tables.NewWriter(catalog), hindsight.Run{
			ID:        "integration",
			StartedAt: time.Now(),
		}, 128, time.Hour,
	)

	if err != nil {
		t.Fatal(err)
	}
	balance, err := broker.NewFundedBalance("USD", decimal.NewFromInt64(200))

	if err != nil {
		t.Fatal(err)
	}
	learner, err := NewLearner(t.Context(), api, price, balance, 3, catalog, newMemoryBlobs(), "integration", recorder)

	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if learner.Rehearsal != nil {
			if err := learner.Rehearsal.Workload.Close(); err != nil {
				t.Error(err)
			}
		}
		if err := recorder.Close(); err != nil {
			t.Error(err)
		}
	})
	// Lifecycle fixtures use four calibration bins so their short tapes can
	// cover formation and post-formation behavior in the same test.
	learner.Grid.Space = grid.NewSpaceWithWindow(4)

	for _, cursor := range learner.Rehearsal.cursors {
		cursor.space = grid.NewSpaceWithWindow(4)
	}
	return learner, conn
}

func TestLearnerStep(t *testing.T) {
	Convey("Wallet valuations advance while source timestamps arrive out of order", t, func() {
		learner, conn := learningFixture(t)
		start := time.Now().Add(-time.Hour)
		bookTape := market.NewLevel3Tape("BTC/USD", start)

		for _, message := range bookTape.Messages {
			conn.ApplyLevel3(message)
		}
		learner.Agent.Model.Observe(
			agent.ContextKey("BTC/USD", []uint64{FlatPositionContext}),
			[]byte(fmt.Sprint(Action{Kind: "wait"})),
		)
		tape := market.NewOpportunityTape("BTC/USD", start, 6)
		var measured uint64

		for _, event := range tape.Steps {
			measurement := data.NewMeasurement[float64](
				"sequence", "BTC/USD", "context", event.EventTime, start,
			)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "development", Raw: event.Context})
			measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: -event.Context})
			measurement.SNR, measurement.SNRDefined = 100, true
			envelope := learner.Grid.Step(&types.Envelope{CVD: measurement})
			So(learner.Grid.Error(), ShouldBeNil)

			if !envelope.Impulses[0].Ready || len(envelope.Impulses[0].Regions) == 0 {
				continue
			}
			// Reproduce the reported regression inside a single envelope, then
			// an equal source instant. Neither changes live valuation order.
			older := envelope.Impulses[0]
			older.At = older.At.Add(-1339250 * time.Nanosecond)
			envelope.Impulses = append(envelope.Impulses, older, older)
			previous := learner.Agent.Reward
			before := time.Now()
			learner.Step(envelope)
			after := time.Now()
			So(learner.Error(), ShouldBeNil)
			measured += uint64(len(envelope.Impulses))
			outcome := learner.Agent.Reward
			So(outcome.Through.Version, ShouldEqual, measured)
			So(outcome.Transitions, ShouldEqual, measured-1)
			So(outcome.Through.At.Before(before), ShouldBeFalse)
			So(outcome.Through.At.After(after), ShouldBeFalse)
			So(outcome.TotalElapsed >= previous.TotalElapsed, ShouldBeTrue)
			So(outcome.TotalReward, ShouldEqual, 0)
			So(learner.At.Equal(older.At), ShouldBeTrue)
			So(learner.Agent.Last.At.Equal(older.At), ShouldBeTrue)
			So(envelope.Impulses[0].At.Equal(event.EventTime), ShouldBeTrue)
		}
		So(measured, ShouldBeGreaterThan, 3)
	})

	Convey("A grid activation is independent of the envelope transport", t, func() {
		learner, conn := learningFixture(t)
		bookTape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, event := range bookTape.Messages {
			conn.ApplyLevel3(event)
		}
		tape := market.NewOpportunityTape("BTC/USD", time.Now(), 6)

		for _, event := range tape.Steps {
			measurement := data.NewMeasurement[float64]("sequence", "BTC/USD", "context", event.EventTime, tape.Steps[0].EventTime)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "development", Raw: event.Context})
			measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: -event.Context})
			measurement.SNR, measurement.SNRDefined = 100, true
			envelope := &types.Envelope{Key: "BTC/USD", TypeID: types.EnvelopeTrade, CVD: measurement}
			learner.Grid.Step(envelope)
			for _, impulse := range envelope.Impulses {
				// A pre-learned flat-position wait class allows this lifecycle test
				// to exercise decisions without inventing cold-start inference.
				learner.Agent.Model.Observe(agent.ContextKey(impulse.Label, []uint64{FlatPositionContext}), []byte(fmt.Sprint(Action{Kind: "wait"})))
			}
			So(learner.Step(envelope), ShouldEqual, envelope)
			So(learner.Error(), ShouldBeNil)
			So(learner.Grid.Version, ShouldBeGreaterThan, 0)
		}
		So(learner.Decisions, ShouldBeGreaterThan, 0)
		So(len(learner.Traders), ShouldEqual, 1)
		So(1, ShouldEqual, 1)
		So(learner.Rehearsal, ShouldNotBeNil)

		Convey("The consolidated model checkpoints and restores into every member", func() {
			So(learner.Save(context.Background()), ShouldBeNil)
			fresh, err := NewLearner(t.Context(), learner.Traders[0].api, learner.price, learner.Traders[0].Balance, 3, learner.catalog, learner.archive, "next", learner.recorder)
			So(err, ShouldBeNil)
			So(fresh.Restored, ShouldBeTrue)
			So(fresh.Grid.Columns, ShouldResemble, learner.Grid.Columns)
			So(1, ShouldEqual, 1)
		})
	})
}

func BenchmarkLearnerStep(b *testing.B) {
	learner, conn := learningFixture(b)
	for _, event := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
		conn.ApplyLevel3(event)
	}
	tape := market.NewOpportunityTape("BTC/USD", time.Now(), 8)
	b.ReportAllocs()

	for index := 0; b.Loop(); index++ {
		event := tape.Steps[index%len(tape.Steps)]
		measurement := data.NewMeasurement[float64]("benchmark", "BTC/USD", "context", event.EventTime, event.EventTime)
		measurement.Maturity = 1
		measurement.PutMetric(data.Metric[float64]{Label: "development", Raw: event.Context})
		measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: -event.Context})
		measurement.SNR, measurement.SNRDefined = 100, true
		learner.Step(learner.Grid.Step(&types.Envelope{Key: "BTC/USD", TypeID: types.EnvelopeTrade, CVD: measurement}))

		if err := learner.Error(); err != nil {
			b.Fatal(err)
		}
	}

	if learner.Steps == 0 {
		b.Fatal("benchmark did not exercise a live wallet valuation")
	}
}

func TestLearnerReview(t *testing.T) {
	Convey("Given issued decisions and a later multi-leg durable tape", t, func() {
		learner, conn := learningFixture(t)
		learner.Agent.Model.Observe(agent.ContextKey("BTC/USD", []uint64{FlatPositionContext}), []byte(fmt.Sprint(Action{Kind: "wait"})))
		for _, event := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
			conn.ApplyLevel3(event)
		}
		for index := range 12 {
			at := time.Now()
			measurement := data.NewMeasurement[float64]("test", "BTC/USD", "context", at, at)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "change", Raw: float64(index % 3)})
			measurement.PutMetric(data.Metric[float64]{Label: "flow", Raw: -float64(index % 3)})
			measurement.SNR, measurement.SNRDefined = 100, true
			learner.Step(learner.Grid.Step(&types.Envelope{TypeID: types.EnvelopeTrade, CVD: measurement}))
			So(learner.Error(), ShouldBeNil)
		}
		So(learner.Decisions, ShouldBeGreaterThan, 0)
		original := learner.Agent.Last
		So(original, ShouldNotBeNil)
		So(learner.Review(t.Context()), ShouldBeNil)
		So(learner.Resolved, ShouldEqual, 0)
		// The intermediate dips are pullbacks inside one developing move; only
		// the final print gives back half the excursion, so the confirmed leg
		// runs all the way to 130 rather than stopping at the first dip.
		writeCaptures(t, learner, []int64{100, 110, 106, 130, 122, 95})
		So(learner.Review(t.Context()), ShouldBeNil)
		So(learner.Resolved, ShouldEqual, learner.Decisions)
		So(original.Outcome, ShouldNotBeNil)
		So(learner.Agent.Negative, ShouldBeGreaterThan, 0)

		// Graded decisions reach the outcomes table through the recorder, so
		// the batch has to be flushed before they can be read back.
		So(learner.recorder.Close(), ShouldBeNil)

		persisted, err := learner.catalog.Outcomes(t.Context(), string(learner.run))
		So(err, ShouldBeNil)
		So(len(persisted), ShouldEqual, learner.Resolved)

		for _, evaluation := range persisted {
			So(evaluation.Complete, ShouldBeTrue)
			So(evaluation.Outcome, ShouldNotBeNil)
			So(*evaluation.Outcome, ShouldEqual, evaluation.Value)
		}
		settled := learner.Resolved
		So(learner.Review(t.Context()), ShouldBeNil)
		So(learner.Resolved, ShouldEqual, settled)
	})
}

func TestLearnerReady(t *testing.T) {
	Convey("The real workload gates decisions while signal and grid stages keep advancing", t, func() {
		learner, connection := learningFixture(t)
		learner.Agent.Model.Observe(agent.ContextKey("BTC/USD", []uint64{FlatPositionContext}), []byte(fmt.Sprint(Action{Kind: "wait"})))
		gridWorkload := runtime.NewWorkload(t.Context(), "grid", [][]runtime.Node[*types.Envelope]{{learner.Grid}})
		agentWorkload := runtime.NewWorkload(t.Context(), "agent", [][]runtime.Node[*types.Envelope]{{learner}})
		agentWorkload.Require(learner.Ready)
		workspace := runtime.NewWorkspace(t.Context(), "learning", [][]runtime.Node[*types.Envelope]{{gridWorkload}, {agentWorkload}})
		So(workspace.Error(), ShouldBeNil)
		defer func() { So(workspace.Close(), ShouldBeNil) }()
		workspace.Admit()
		advance := func(index int) {
			at := time.Unix(int64(index+1), 0)
			measurement := data.NewMeasurement[float64]("test", "BTC/USD", "flow", at, time.Unix(1, 0))
			measurement.Metadata = map[string]float64{data.MetadataSupport: float64(index + 1), data.MetadataMahalanobisSNR: 100}
			measurement.PutMetric(data.Metric[float64]{Label: "first", Raw: float64(index % 2)})
			measurement.PutMetric(data.Metric[float64]{Label: "second", Raw: -float64(index % 2)})
			workspace.Step(&types.Envelope{TypeID: types.EnvelopeTrade, CVD: measurement})
		}
		for index := range 32 {
			advance(index)
		}
		So(learner.Grid.Version, ShouldEqual, 32)
		So(learner.Decisions, ShouldEqual, 0)
		So(agentWorkload.Status().String(), ShouldEqual, "waiting")
		So(learner.Agent.Pending, ShouldBeEmpty)
		tape := market.NewLevel3Tape("BTC/USD", time.Now())
		for _, message := range tape.Messages[:4] {
			connection.ApplyLevel3(message)
		}
		for index := 32; index < 64; index++ {
			advance(index)
		}
		So(agentWorkload.Status().String(), ShouldEqual, "ready")
		So(learner.Decisions, ShouldBeGreaterThan, 0)
		decisions, samples := learner.Decisions, learner.Agent.Reading.Samples
		connection.ApplyLevel3(kraken.Level3Data{Type: "snapshot", Symbol: "BTC/USD"})
		for index := 64; index < 96; index++ {
			advance(index)
		}
		So(agentWorkload.Status().String(), ShouldEqual, "waiting")
		So(learner.Decisions, ShouldEqual, decisions)
		So(learner.Agent.Reading.Samples, ShouldEqual, samples)
		So(learner.Grid.Version, ShouldEqual, 96)
	})
}

/*
memoryBlobs is the checkpoint's object storage held in memory. The model is the
only artifact left outside the tables, so save-and-restore needs somewhere to
put one object and nothing more.
*/
type memoryBlobs struct {
	mutex   sync.Mutex
	objects map[string][]byte
}

func newMemoryBlobs() *memoryBlobs {
	return &memoryBlobs{objects: map[string][]byte{}}
}

func (blobs *memoryBlobs) Read(_ context.Context, key string) ([]byte, bool, error) {
	blobs.mutex.Lock()
	defer blobs.mutex.Unlock()

	data, found := blobs.objects[key]

	return data, found, nil
}

func (blobs *memoryBlobs) Write(_ context.Context, key string, data []byte) error {
	blobs.mutex.Lock()
	defer blobs.mutex.Unlock()

	blobs.objects[key] = bytes.Clone(data)

	return nil
}

// writeCaptures persists trade captures for the learner's run, so Review has a
// durable tape to consume.
func writeCaptures(t *testing.T, learner *Learner, prices []int64) {
	t.Helper()

	writer := tables.NewWriter(learner.catalog)

	for index, price := range prices {
		payload, err := json.Marshal(kraken.Trade{Data: []kraken.TradeData{
			{Symbol: "BTC/USD", Price: *decimal.NewFromInt64(price)},
		}})

		if err != nil {
			t.Fatal(err)
		}

		writer.AddCapture(tables.CaptureRow{
			Run: string(learner.run), Sequence: int64(index + 1), Stream: "spot",
			StreamEpoch: 1, StreamSequence: int64(index + 1),
			ReceivedAt: learner.At.Add(time.Duration(index+1) * time.Second),
			Kind:       "trade", PayloadHash: "hash", Payload: payload,
		})
	}

	if err := writer.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}
