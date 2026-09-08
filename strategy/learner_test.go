package strategy

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/theapemachine/symm/tests/market"
	"github.com/theapemachine/symm/tests/venue"
	"github.com/theapemachine/symm/types"
)

// learningFixture supplies the real book reducer, pricing, positions, balance,
// associative population and recorder. Only external venue I/O is substituted.
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
		if err := recorder.Close(); err != nil {
			t.Error(err)
		}
	})
	return learner, conn
}

func TestLearnerStep(t *testing.T) {
	Convey("A grid activation is independent of the envelope transport", t, func() {
		learner, conn := learningFixture(t)
		bookTape := market.NewLevel3Tape("BTC/USD", time.Now())

		for _, event := range bookTape.Messages {
			conn.ApplyLevel3(event)
		}
		tape := market.NewOpportunityTape("BTC/USD", time.Now(), 6)

		for index, event := range tape.Steps {
			measurement := data.NewMeasurement[float64]("sequence", "BTC/USD", "context", event.EventTime, tape.Steps[0].EventTime)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "development", Raw: event.Context})
			envelope := &types.Envelope{Key: "BTC/USD", TypeID: types.EnvelopeTrade, CVD: measurement}
			So(learner.Step(envelope), ShouldEqual, envelope)
			So(learner.Error(), ShouldBeNil)
			So(learner.Steps, ShouldEqual, index+1)
		}
		So(learner.Decisions, ShouldBeGreaterThan, 0)
		So(len(learner.Traders), ShouldEqual, 1)
		So(len(learner.Population.Agents), ShouldEqual, 1)
		So(learner.Rehearsal, ShouldNotBeNil)

		Convey("The consolidated model checkpoints and restores into every member", func() {
			So(learner.Population.Save(context.Background(), learner.Checkpoint), ShouldBeNil)
			fresh, err := NewLearner(t.Context(), learner.Traders[0].api, learner.price, learner.Traders[0].Balance, 3, learner.catalog, learner.Checkpoint.Store, "next", learner.recorder)
			So(err, ShouldBeNil)
			So(fresh.Restored, ShouldBeTrue)
			So(fresh.Population.Grid.Columns, ShouldResemble, learner.Population.Grid.Columns)
			So(len(fresh.Population.Agents), ShouldEqual, 1)
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
		learner.Step(&types.Envelope{Key: "BTC/USD", TypeID: types.EnvelopeTrade, CVD: measurement})

		if err := learner.Error(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLearnerReview(t *testing.T) {
	Convey("Given issued decisions and a later multi-leg durable tape", t, func() {
		learner, conn := learningFixture(t)
		for _, event := range market.NewLevel3Tape("BTC/USD", time.Now()).Messages[:4] {
			conn.ApplyLevel3(event)
		}
		for index := range 12 {
			at := time.Now()
			measurement := data.NewMeasurement[float64]("test", "BTC/USD", "context", at, at)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "change", Raw: float64(index % 3)})
			learner.Step(&types.Envelope{TypeID: types.EnvelopeTrade, CVD: measurement})
			So(learner.Error(), ShouldBeNil)
		}
		So(learner.Decisions, ShouldBeGreaterThan, 0)
		original := learner.Population.Agents[0].Last
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
		So(learner.Population.Agents[0].Negative, ShouldBeGreaterThan, 0)

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

func TestLearnerReleasesForcedDecisions(t *testing.T) {
	Convey("A decision with no alternative is released instead of graded", t, func() {
		learner, _ := learningFixture(t)

		// No book was applied, so every symbol offers waiting and nothing else.
		for index := range 6 {
			at := time.Now()
			measurement := data.NewMeasurement[float64]("test", "BTC/USD", "context", at, at)
			measurement.Maturity = 1
			measurement.PutMetric(data.Metric[float64]{Label: "change", Raw: float64(index % 3)})
			learner.Step(&types.Envelope{TypeID: types.EnvelopeTrade, CVD: measurement})
			So(learner.Error(), ShouldBeNil)
		}
		So(learner.Decisions, ShouldBeGreaterThan, 0)

		for _, trader := range learner.Traders {
			for _, evaluations := range trader.Evaluations {
				for _, evaluation := range evaluations {
					So(evaluation.Forced, ShouldBeTrue)
				}
			}
		}
		writeCaptures(t, learner, []int64{100, 130, 95})
		So(learner.Review(t.Context()), ShouldBeNil)

		Convey("None of them became training evidence", func() {
			So(learner.Forced, ShouldEqual, learner.Decisions)
			So(learner.Resolved, ShouldEqual, 0)

			for _, member := range learner.Population.Agents {
				So(member.Positive, ShouldEqual, 0)
				So(member.Negative, ShouldEqual, 0)
				So(member.Pending, ShouldBeEmpty)
			}
		})
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
