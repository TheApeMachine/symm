package pumpdump

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

func TestMeasure(t *testing.T) {
	Convey("Given pumpdump trade observations on one symbol", t, func() {
		signal := &Signal{
			ctx:  context.Background(),
			algo: equation.NewIgnition(128),
		}
		market := types.NewSymbol("BTC/USD", nil)
		market.AppendTrade(pumpdumpTrade(1, "buy", 100, time.Unix(1_700_002_200, 0).UTC()))

		measurements := signal.Measure(market)

		Convey("It should keep the signal contract while nomagique rejects incomplete quote evidence", func() {
			So(measurements, ShouldBeEmpty)
			So(signal.Name(), ShouldEqual, string(types.SourcePumpDump))
			So(signal.Type(), ShouldEqual, types.SourcePumpDump)
		})
	})
}

func pumpdumpTrade(
	id int64,
	side string,
	price float64,
	at time.Time,
) kraken.TradeData {
	return kraken.TradeData{
		Symbol: "BTC/USD", Side: side, Price: *decimal.NewFromFloat64(price),
		Qty: 20, TradeID: id, Timestamp: at,
	}
}
