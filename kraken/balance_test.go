package kraken

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
)

/*
TestNewPaperBalance proves the paper CLI wallet payload decodes into the same
asset-to-decimal map shape used by Kraken's real balance client.
*/
func TestNewPaperBalance(t *testing.T) {
	Convey("Given a paper balance payload", t, func() {
		payload := []byte(`{"balances":{"BTC":{"available":0.00007585,"reserved":0.0,"total":0.00007585},"TAO":{"available":0.0254,"reserved":0.0,"total":0.0254},"USD":{"available":179.2471020758235,"reserved":0.0,"total":179.2471020758235}},"mode":"paper"}`)

		Convey("Decoding it should preserve native decimal totals by asset", func() {
			balance := NewPaperBalance(payload)
			totals := balance.Totals()

			So(balance.Mode, ShouldEqual, "paper")
			So(balance.Balances["BTC"].Available.String(), ShouldEqual, "0.00007585")
			So(balance.Balances["BTC"].Reserved.String(), ShouldEqual, "0.0")
			So(totals["BTC"].String(), ShouldEqual, "0.00007585")
			So(totals["TAO"].String(), ShouldEqual, "0.0254")
			So(totals["USD"].String(), ShouldEqual, "179.2471020758235")
		})
	})
}

/*
TestNewBalanceFromMap proves the paper wallet dump is reshaped into the balance
websocket frame with total, available, and reserved values intact.
*/
func TestNewBalanceFromMap(t *testing.T) {
	Convey("Given a paper balance model", t, func() {
		model := datura.Map[any]{
			"balances": map[string]any{
				"BTC": map[string]any{
					"available": 0.00007585,
					"reserved":  0.0,
					"total":     0.00007585,
				},
			},
		}

		Convey("Reshaping it should populate the websocket balance row", func() {
			balance := NewBalanceFromMap(model)

			So(balance.Channel, ShouldEqual, "balances")
			So(balance.Type, ShouldEqual, "snapshot")
			So(len(balance.Data), ShouldEqual, 1)
			So(balance.Data[0].Asset, ShouldEqual, "BTC")
			So(balance.Data[0].Balance.String(), ShouldEqual, "0.00007585")
			So(balance.Data[0].Available.String(), ShouldEqual, "0.00007585")
			So(balance.Data[0].Reserved.String(), ShouldEqual, "0")
		})
	})
}

/*
BenchmarkNewPaperBalance measures the paper wallet decode path because balance
fetching is a data-processing path that should stay cheap under repeated polls.
*/
func BenchmarkNewPaperBalance(b *testing.B) {
	payload := []byte(`{"balances":{"BTC":{"available":0.00007585,"reserved":0.0,"total":0.00007585},"TAO":{"available":0.0254,"reserved":0.0,"total":0.0254},"USD":{"available":179.2471020758235,"reserved":0.0,"total":179.2471020758235}},"mode":"paper"}`)

	b.ReportAllocs()

	for b.Loop() {
		NewPaperBalance(payload)
	}
}
