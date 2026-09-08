package strategy

import (
 "context"
 "testing"
 "time"

 "github.com/krakenfx/api-go/v2/pkg/decimal"
 "github.com/krakenfx/api-go/v2/pkg/spot"
 . "github.com/smartystreets/goconvey/convey"
 "github.com/theapemachine/symm/broker"
 "github.com/theapemachine/symm/hindsight"
 "github.com/theapemachine/symm/hindsight/recording"
 "github.com/theapemachine/symm/kraken"
 "github.com/theapemachine/symm/kraken/websocket"
 "github.com/theapemachine/symm/nomagique/data"
 "github.com/theapemachine/symm/tests/market"
 "github.com/theapemachine/symm/tests/venue"
 "github.com/theapemachine/symm/types"
 "gocloud.dev/blob/memblob"
)

// learningFixture supplies the real book reducer, pricing, positions, balance,
// associative population and recorder. Only external venue I/O is substituted.
func learningFixture(t testing.TB) (*Learner, *venue.Conn) {
 t.Helper()
 conn := venue.NewConn()
 api := websocket.NewAPI(t.Context(), conn, conn)
 api.Normalizer().Update(&spot.AssetsManagerUpdate{
  NewAssets: map[string]spot.AssetInfo{"BTC": {AltName: "BTC", Decimals: 8}, "USD": {AltName: "USD", Decimals: 2}},
  NewPairs: map[string]spot.AssetPair{"BTCUSD": {WSName: "BTC/USD", Base: "BTC", Quote: "USD", PairDecimals: 2, LotDecimals: 8, LotMultiplier: 1}},
 })
 instrument := broker.NewInstrumentWithQuote("USD")
 instrument.Cache([]kraken.InstrumentPair{{Symbol: "BTC/USD", Base: "BTC", Quote: "USD", Status: "online", QtyMin: venue.Decimal("0.0001"), CostMin: venue.Decimal("0.5"), QtyIncrement: venue.Decimal("0.00000001")}})
 price := broker.NewPrice(api, instrument)
 price.SetFee("BTC/USD", kraken.TradeVolumeFee{Fee: venue.Decimal("0.25")})
 price.Update(&kraken.TickerData{Symbol: "BTC/USD", Last: venue.Decimal("100"), Bid: venue.Decimal("99"), Ask: venue.Decimal("101")})
 archive := memblob.OpenBucket(nil)
 recorder, err := recording.NewSession(t.Context(), archive, hindsight.Run{ID: "integration", StartedAt: time.Now()}, 16384, 128, time.Hour)

 if err != nil { t.Fatal(err) }
 balance, err := broker.NewFundedBalance("USD", decimal.NewFromInt64(200))

 if err != nil { t.Fatal(err) }
 learner, err := NewLearner(t.Context(), api, price, balance, 3, archive, "integration", recorder)

 if err != nil { t.Fatal(err) }
 t.Cleanup(func() {
  if err := recorder.Close(); err != nil { t.Error(err) }

  if err := archive.Close(); err != nil { t.Error(err) }
 })
 return learner, conn
}

func TestLearnerStep(t *testing.T) {
 Convey("A grid activation is independent of the envelope transport", t, func() {
  learner, conn := learningFixture(t)
  bookTape := market.NewLevel3Tape("BTC/USD", time.Now())

  for _, event := range bookTape.Messages { conn.ApplyLevel3(event) }
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
  So(learner.Traders[0].Balance, ShouldNotEqual, learner.Traders[1].Balance)
  So(learner.Population.Agents[0].Model == learner.Population.Agents[1].Model, ShouldBeFalse)

  Convey("The consolidated model checkpoints and restores into every member", func() {
   So(learner.Population.Save(context.Background(), learner.Checkpoint), ShouldBeNil)
   fresh, err := NewLearner(t.Context(), learner.Traders[0].api, learner.price, learner.Traders[0].Balance, 3, learner.Checkpoint.Bucket, "next", learner.recorder)
   So(err, ShouldBeNil)
   So(fresh.Restored, ShouldBeTrue)
   So(fresh.Population.Grid.Columns, ShouldResemble, learner.Population.Grid.Columns)
   So(fresh.Population.Agents[0].Model == fresh.Population.Agents[1].Model, ShouldBeFalse)
  })
 })
}

func BenchmarkLearnerStep(b *testing.B) {
 learner, _ := learningFixture(b)
 tape := market.NewOpportunityTape("BTC/USD", time.Now(), 8)
 b.ReportAllocs()
 b.ResetTimer()

 for index := 0; index < b.N; index++ {
  event := tape.Steps[index%len(tape.Steps)]
  measurement := data.NewMeasurement[float64]("benchmark", "BTC/USD", "context", event.EventTime, event.EventTime)
  measurement.Maturity = 1
  measurement.PutMetric(data.Metric[float64]{Label: "development", Raw: event.Context})
  learner.Step(&types.Envelope{Key: "BTC/USD", TypeID: types.EnvelopeTrade, CVD: measurement})

  if err := learner.Error(); err != nil { b.Fatal(err) }
 }
}
