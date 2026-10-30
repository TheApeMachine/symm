package broker

import (
	"testing"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/kraken"
)

func TestTickFromAggregateKey(t *testing.T) {
	Convey("tickFromAggregateKey parses the trailing tick segment", t, func() {
		tick, ok := tickFromAggregateKey("workspace/tick/aggregate/0000000000000000001/0000000000000000042")
		So(ok, ShouldBeTrue)
		So(tick, ShouldEqual, 42)
	})
}

func TestLastTradePrice(t *testing.T) {
	Convey("lastTradePrice returns the latest print for the symbol", t, func() {
		value := data.NewValue("workspace", "tick", "aggregate")
		value.Append([]kraken.TradeData{
			{Symbol: "BTC/USD", Price: dec("100")},
			{Symbol: "ETH/USD", Price: dec("10")},
			{Symbol: "BTC/USD", Price: dec("105")},
		}, "spot:trade", "trade")

		price := lastTradePrice(value, "BTC/USD")
		So(price, ShouldNotBeNil)
		So(price.String(), ShouldEqual, "105")
	})
}

func TestWidenTrailForRegret(t *testing.T) {
	Convey("widenTrailForRegret widens the trail when episodes left profit on the table", t, func() {
		trail := widenTrailForRegret(0.4, []episode{
			{regretPct: 0.2},
			{regretPct: 0.6},
		})
		So(trail, ShouldEqual, 0.7)
	})
}

func TestEncodeDecodePolicyRecord(t *testing.T) {
	Convey("encode and decode round-trip a policy record", t, func() {
		record := &policyRecord{
			episodes: []episode{
				{
					trace:      []float64{0, 0.5, 0.3},
					banked:     0.3,
					regretPct:  0.2,
					shadowBars: 4,
				},
			},
			policy: learnedExitPolicy{
				invalidationCut: 0.1,
				trailingStop:    0.2,
				samples:         3,
			},
		}
		record.rows = policyRows(record.episodes)

		value := data.NewValue("model", "parameters", "exitpolicy")
		value.Append(encodePolicyRecord(record), "exit_policy")

		restored := decodePolicyRecord(value)
		So(restored, ShouldNotBeNil)
		So(restored.policy.trailingStop, ShouldEqual, 0.2)
		So(len(restored.episodes), ShouldEqual, 1)
		So(restored.episodes[0].regretPct, ShouldEqual, 0.2)
		So(restored.episodes[0].shadowBars, ShouldEqual, 4)
	})
}

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}

	return *d
}
