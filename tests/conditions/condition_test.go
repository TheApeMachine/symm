package conditions_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/tests/mockapi"
)

/*
TestLevel3Path proves the producer survives Kraken's production SDK lifecycle
and checksum validation rather than merely emitting plausible JSON.
*/
func TestLevel3Path(t *testing.T) {
	Convey("Given an explicit moving L3 population", t, func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		mock := mockapi.NewMockAPI()
		api := websocket.NewAPI(ctx, mock.Public(), mock.Private(), nil)
		defer api.Close()
		live := websocket.New(ctx, nil, true, websocket.Level3WebSocketURL)
		defer live.Close()
		api.AttachLevel3(live)
		So(live.ApplyLevel3([]byte(`{
			"method":"subscribe",
			"params":{"channel":"level3","symbol":["MATIC/USD"],"depth":10}
		}`)), ShouldBeNil)
		startedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		path := conditions.Level3Path(
			[]float64{0.5667, 0.5677},
			[][]float64{{100, 40}, {120, 50}},
			[][]float64{{80, 30}, {60, 20}},
			[]time.Time{startedAt, startedAt.Add(time.Second)},
		)

		for frame := range path.Frames() {
			if frame.Channel == "level3" {
				So(live.ApplyLevel3(frame.Payload), ShouldBeNil)
			}
		}

		Convey("Then the final checksum-valid touch matches the requested midpoint", func() {
			midpoint := 0.0
			found := api.PeekBook(conditions.Subject(), func(symbolBook *book.Book) {
				midpoint = symbolBook.Midpoint().Float64()
			})
			So(found, ShouldBeTrue)
			So(midpoint, ShouldAlmostEqual, 0.5677)
		})
	})
}

/*
TestPumpDump proves the producer emits coherent Kraken ticker state and the
intended recurring pump-and-dump phases.
*/
func TestPumpDump(t *testing.T) {
	Convey("Given the synthetic pump-and-dump condition", t, func() {
		rows := make([]kraken.TickerData, 0)
		trades := make([]kraken.TradeData, 0)
		books := make([]kraken.BookData, 0)

		for frame := range conditions.PumpDump().Frames() {
			switch frame.Channel {
			case "ticker":
				rows = append(rows, kraken.NewTicker(frame.Payload).Data...)
			case "trade":
				trades = append(trades, kraken.NewTrade(frame.Payload).Data...)
			case "book":
				books = append(books, kraken.NewBook(frame.Payload).Data...)
			}
		}

		Convey("Then Kraken ticker state stays coherent through every leg", func() {
			So(len(rows), ShouldEqual, 15)
			So(len(trades), ShouldEqual, len(rows))
			So(len(books), ShouldEqual, len(rows))
			opening := rows[0].Last.Float64()
			previousVolume := 0.0
			totalQuantity := 0.0

			for index, row := range rows {
				last := row.Last.Float64()
				low := row.Low.Float64()
				high := row.High.Float64()
				change := row.Change.Float64()
				bestBid := 0.0
				bestAsk := math.MaxFloat64

				for _, level := range books[index].Bids {
					if level.Qty > 0 {
						bestBid = max(bestBid, level.Price.Float64())
					}
				}

				for _, level := range books[index].Asks {
					if level.Qty > 0 {
						bestAsk = min(bestAsk, level.Price.Float64())
					}
				}

				totalQuantity += trades[index].Qty

				So(row.Bid.Float64(), ShouldBeLessThan, last)
				So(row.Ask.Float64(), ShouldBeGreaterThan, last)
				So(row.Volume, ShouldBeGreaterThanOrEqualTo, previousVolume)
				So(row.Volume-previousVolume, ShouldAlmostEqual, trades[index].Qty)
				So(row.Volume, ShouldAlmostEqual, totalQuantity)
				So(trades[index].Price.Float64(), ShouldAlmostEqual, last)
				So(trades[index].Timestamp, ShouldResemble, row.Timestamp)
				So(books[index].Timestamp, ShouldResemble, row.Timestamp)
				So(bestBid, ShouldAlmostEqual, row.Bid.Float64())
				So(bestAsk, ShouldAlmostEqual, row.Ask.Float64())
				So(low, ShouldBeLessThanOrEqualTo, last)
				So(high, ShouldBeGreaterThanOrEqualTo, last)
				So(row.Vwap, ShouldBeGreaterThanOrEqualTo, low)
				So(row.Vwap, ShouldBeLessThanOrEqualTo, high)
				So(change, ShouldAlmostEqual, last-opening)
				So(row.ChangePct, ShouldAlmostEqual, change/opening*100)
				previousVolume = row.Volume
			}
		})

		Convey("Then the tape contains the intended market phases", func() {
			baseline := rows[len(rows)-7]
			compression := rows[len(rows)-6]
			ignition := rows[len(rows)-5]
			continuation := rows[len(rows)-4]
			rejection := rows[len(rows)-3]
			recoiled := rows[len(rows)-2]
			reignition := rows[len(rows)-1]
			spread := func(row kraken.TickerData) float64 {
				return row.Ask.Float64() - row.Bid.Float64()
			}
			lift := func(current kraken.TickerData, prior kraken.TickerData) float64 {
				return current.Volume - prior.Volume
			}

			So(spread(compression), ShouldBeLessThan, spread(baseline))
			So(lift(ignition, compression),
				ShouldBeGreaterThan, lift(compression, baseline))
			So(ignition.Last.Float64(), ShouldBeGreaterThan, compression.Last.Float64())
			So(continuation.Last.Float64(), ShouldBeGreaterThan, ignition.Last.Float64())
			So(rejection.Last.Float64(), ShouldBeLessThan, continuation.Last.Float64())
			So(trades[len(trades)-3].Side, ShouldEqual, "sell")
			So(lift(rejection, continuation),
				ShouldBeLessThan, lift(continuation, ignition))
			So(spread(rejection), ShouldBeGreaterThan, spread(baseline))
			So(spread(recoiled), ShouldBeLessThan, spread(baseline))
			So(reignition.Last.Float64(), ShouldBeGreaterThan, recoiled.Last.Float64())
			So(lift(reignition, recoiled),
				ShouldBeGreaterThan, lift(ignition, compression))
			So(math.IsNaN(reignition.Vwap), ShouldBeFalse)
		})
	})
}

func TestBuilders(t *testing.T) {
	Convey("Given named market conditions", t, func() {
		markets := []struct {
			name   string
			market *tests.Market
		}{
			{"calm", conditions.Calm(4)},
			{"pump_dump", conditions.PumpDump()},
			{"drawdown", conditions.Drawdown(4, 0.1, 2)},
			{"reversal", conditions.Reversal(4, 2, 0.05)},
			{"aggression", conditions.Aggression(4, 2, 4)},
			{"decay", conditions.Decay(4, 0, 0.8)},
			{"imbalance", conditions.Imbalance(4, 0, 3, 0.3)},
			{"lag", conditions.Lag(4, 2)},
			{"herd", conditions.Herd(4)},
			{"noise", conditions.Noise(4)},
			{"alpha", conditions.Alpha(4)},
			{"divergence", conditions.Divergence(4)},
			{"slump", conditions.Slump(4)},
			{"stall", conditions.Stall(8)},
			{"phantom_drawdown", conditions.PhantomDrawdown(4, 1, 0.015)},
			{"calibrated_lift", conditions.CalibratedLift(4, 1, 1.04)},
		}

		for _, entry := range markets {
			count := 0

			for range entry.market.Frames() {
				count++
			}

			So(count, ShouldBeGreaterThan, 4)
		}

		So(conditions.Subject(), ShouldEqual, "MATIC/USD")
	})
}
