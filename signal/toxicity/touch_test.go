package toxicity

import (
	"context"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

func TestWithinTouchToleranceWithinTick(t *testing.T) {
	Convey("Given a trade and touch price separated by one tick", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		touchPrice := decimal.NewFromFloat64(100.0001)
		increment := decimal.NewFromFloat64(0.0001)

		Convey("Then withinTouchTolerance should accept it as a fill", func() {
			So(withinTouchTolerance(tradePrice, touchPrice, increment), ShouldBeTrue)
		})
	})
}

func TestWithinTouchToleranceRejectsBeyondTick(t *testing.T) {
	Convey("Given a trade further than one tick from the touch", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		touchPrice := decimal.NewFromFloat64(100.0003)
		increment := decimal.NewFromFloat64(0.0001)

		Convey("Then withinTouchTolerance should reject it", func() {
			So(withinTouchTolerance(tradePrice, touchPrice, increment), ShouldBeFalse)
		})
	})
}

func TestWithinTouchToleranceFallsBackToExactWhenIncrementMissing(t *testing.T) {
	Convey("Given no usable price increment", t, func() {
		tradePrice := *decimal.NewFromFloat64(100)
		increment := decimal.NewFromFloat64(0)

		Convey("Then withinTouchTolerance should fall back to exact equality", func() {
			So(withinTouchTolerance(tradePrice, decimal.NewFromFloat64(100), increment), ShouldBeTrue)
			So(withinTouchTolerance(tradePrice, decimal.NewFromFloat64(100.0001), increment), ShouldBeFalse)
		})
	})
}

func TestAttributeTouchSideOneTickSpread(t *testing.T) {
	Convey("Given a one-tick spread", t, func() {
		increment := decimal.NewFromFloat64(0.0001)
		bidPrice := decimal.NewFromFloat64(100)
		askPrice := decimal.NewFromFloat64(100.0001)

		Convey("Then a trade at the ask attributes ask only", func() {
			tradePrice := *decimal.NewFromFloat64(100.0001)
			So(
				attributeTouchSide(tradePrice, bidPrice, askPrice, increment),
				ShouldEqual,
				touchSideAsk,
			)
		})

		Convey("Then a trade at the bid attributes bid only", func() {
			tradePrice := *decimal.NewFromFloat64(100)
			So(
				attributeTouchSide(tradePrice, bidPrice, askPrice, increment),
				ShouldEqual,
				touchSideBid,
			)
		})

		Convey("Then a trade equidistant from both sides prefers ask", func() {
			wideAsk := decimal.NewFromFloat64(100.0002)
			tradePrice := *decimal.NewFromFloat64(100.0001)
			So(
				attributeTouchSide(tradePrice, bidPrice, wideAsk, increment),
				ShouldEqual,
				touchSideAsk,
			)
		})
	})
}

func TestAttributeTouchSideSessionSeedPrice(t *testing.T) {
	Convey("Given the session seed touch and first fixture trade price", t, func() {
		payload := tradeFixturePayload()
		tradeRow := kraken.NewTrade(payload).Data[0]
		bidPrice := tradeRow.Price
		askPrice := decimal.NewFromFloat64(tradeRow.Price.Float64() + 0.0001)
		increment := decimal.NewFromFloat64(0.0001)

		Convey("Then the seeded trade attributes to the bid touch only", func() {
			So(
				attributeTouchSide(tradeRow.Price, &bidPrice, askPrice, increment),
				ShouldEqual,
				touchSideBid,
			)
		})
	})
}

func TestCalculateOneTickSpreadDoesNotDoubleCountExecution(t *testing.T) {
	Convey("Given a one-tick book and one ask-side trade", t, func() {
		symbol := "BTC/USD"
		increment := decimal.NewFromFloat64(0.0001)
		eventAt := time.Unix(100, 0).UTC()
		api := websocket.NewAPI(context.Background(), nil, nil, nil)
		live := websocket.New(context.Background(), nil, true, websocket.Level3WebSocketURL)
		api.AttachLevel3(live)

		bid := decimal.NewFromFloat64(100)
		ask := decimal.NewFromFloat64(100.0001)
		live.SeedTouchDecimals(symbol, bid, ask, 1000, eventAt)

		signal := &Signal{
			ctx:        context.Background(),
			level3:     NewLevel3(api),
			priorTouch: map[string]touchSnapshot{},
		}

		frame := &types.MarketFrame{
			Trades: []kraken.TradeData{{
				Symbol:    symbol,
				Price:     *ask,
				Qty:       1,
				Timestamp: eventAt,
			}},
			Books: []kraken.BookData{{
				Symbol:         symbol,
				PriceIncrement: increment,
			}},
			CrossSection: types.NewCrossSection(),
		}

		measurements, err := signal.Calculate(frame)

		Convey("Then fill evidence credits one side only", func() {
			So(err, ShouldBeNil)

			_, hasBuyFill := latestMetric(measurements, types.MetricFillVolume, types.SideBuy)
			_, hasSellFill := latestMetric(measurements, types.MetricFillVolume, types.SideSell)

			So(hasBuyFill, ShouldBeFalse)
			So(hasSellFill, ShouldBeTrue)
		})
	})
}
