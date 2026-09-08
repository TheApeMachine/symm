package hindsight

import (
 "encoding/json"
 "testing"
 "time"

 . "github.com/smartystreets/goconvey/convey"
 "github.com/theapemachine/symm/kraken"
 "github.com/theapemachine/symm/tests/market"
 "github.com/krakenfx/api-go/v2/pkg/decimal"
)

func TestTapeStep(t *testing.T) {
 Convey("Durable directional legs require a subsequent reversal", t, func() {
  var tape Tape
  var completed []Leg
  fixture := market.NewOpportunityTape("BTC/USD", time.Unix(100, 0), 4)

  for index, step := range fixture.Steps {
   payload, err := json.Marshal(kraken.Trade{Data: []kraken.TradeData{{Symbol: fixture.Symbol, Price: *decimal.NewFromFloat64(step.ExecutableBid)}}})
   So(err, ShouldBeNil)
   So(tape.Step(RawFrame{Kind: "trade", ReceivedAt: step.EventTime, Payload: payload}, func(leg Leg) error { completed = append(completed, leg); return nil }), ShouldBeNil)

   if index == 0 { So(completed, ShouldBeEmpty) }
  }
  So(len(completed), ShouldBeGreaterThan, 1)

  for _, leg := range completed { So(leg.ConfirmedAt.After(leg.Through), ShouldBeTrue) }
  So(tape.Step(RawFrame{Kind: "trade", Payload: []byte("broken")}, func(Leg) error { return nil }), ShouldNotBeNil)
 })
}
