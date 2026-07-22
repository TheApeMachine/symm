package tests

import (
	"encoding/json"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

/*
TestValidator_Validate proves a rejected frame cannot mutate the reconstructed
book, checksum, or ticker state used by the next coherent frame.
*/
func TestValidator_Validate(t *testing.T) {
	Convey("Given a valid snapshot followed by one market update", t, func() {
		market := NewMarket(t.Context(), 1)
		market.signal.Bootstrap()
		snapshot, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		validator := NewValidator()
		So(validator.Validate(snapshot), ShouldBeNil)
		So(market.signal.Apply(MarketStep{
			Advance: time.Second,
			Actions: []MarketAction{{
				Kind: MarketTrade, Symbol: "SIM1/USD", Side: "buy", Qty: 1,
			}},
		}), ShouldBeNil)
		update, err := market.read(
			market.ticker,
			market.trade,
			market.book,
			market.level3,
		)
		So(err, ShouldBeNil)
		validLevel3 := append([]byte(nil), update.level3...)
		var corrupted map[string]any
		So(json.Unmarshal(update.level3, &corrupted), ShouldBeNil)
		corrupted["data"].([]any)[0].(map[string]any)["checksum"] = 1
		update.level3, err = json.Marshal(corrupted)
		So(err, ShouldBeNil)

		Convey("A bad checksum should not poison the valid retry", func() {
			So(validator.Validate(update), ShouldNotBeNil)
			update.level3 = validLevel3
			So(validator.Validate(update), ShouldBeNil)
		})
	})
}
