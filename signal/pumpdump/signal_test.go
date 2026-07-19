package pumpdump

import (
	"context"
	"iter"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/tests"
	"github.com/theapemachine/symm/tests/conditions"
	"github.com/theapemachine/symm/types"
)

/*
drive replays a ticker timeline through Calculate one frame at a time, warming
the shared ignition state exactly as the live feed does, and returns the last
frame's measurements.
*/
func drive(
	signal *Signal,
	frames iter.Seq[tests.Frame],
) ([]*types.Measurement, error) {
	var last []*types.Measurement

	for frame := range frames {
		marketFrame := &types.MarketFrame{
			CrossSection: types.NewCrossSection(),
		}

		switch frame.Channel {
		case "ticker":
			marketFrame.Tickers = kraken.NewTicker(frame.Payload).Data
			marketFrame.Advanced = types.StreamTicker
		case "trade":
			marketFrame.Trades = kraken.NewTrade(frame.Payload).Data
			marketFrame.Advanced = types.StreamTrade
		case "book":
			marketFrame.Books = kraken.NewBook(frame.Payload).Data
			marketFrame.Advanced = types.StreamBook
			increment := krakendecimal.NewFromFloat64(0.0001)

			for index := range marketFrame.Books {
				marketFrame.Books[index].PriceIncrement = increment
			}
		default:
			continue
		}

		measurements, err := signal.Calculate(marketFrame)

		if err != nil {
			return nil, err
		}

		if len(measurements) > 0 {
			last = measurements
		}
	}

	return last, nil
}

func newSignal() *Signal {
	return NewSignal(context.Background(), nil, nil)
}

/*
sourceFrame deliberately permits ticker summaries to disagree with executed
trades and book touch so Calculate tests can prove which stream is authoritative.
*/
func sourceFrame(
	at time.Time,
	price float64,
	tradeQuantity float64,
	tickerVolume float64,
	tickerSpread float64,
	bookSpread float64,
) *types.MarketFrame {
	last := krakendecimal.NewFromFloat64(price)
	tickerBid := krakendecimal.NewFromFloat64(price - tickerSpread/2)
	tickerAsk := krakendecimal.NewFromFloat64(price + tickerSpread/2)
	bookBid := krakendecimal.NewFromFloat64(price - bookSpread/2)
	bookAsk := krakendecimal.NewFromFloat64(price + bookSpread/2)
	increment := krakendecimal.NewFromFloat64(0.1)
	trades := make([]kraken.TradeData, 0, 1)

	if tradeQuantity > 0 {
		trades = append(trades, kraken.TradeData{
			Symbol:    "BTC/USD",
			Side:      "buy",
			Price:     *last,
			Qty:       tradeQuantity,
			Timestamp: at,
		})
	}

	return &types.MarketFrame{
		Tickers: []kraken.TickerData{{
			Symbol:    "BTC/USD",
			Bid:       tickerBid,
			Ask:       tickerAsk,
			Last:      last,
			Volume:    tickerVolume,
			Timestamp: at,
		}},
		Trades: trades,
		Books: []kraken.BookData{{
			Symbol:         "BTC/USD",
			Type:           "snapshot",
			PriceIncrement: increment,
			Bids: []kraken.BookLevel{{
				Price: *bookBid,
				Qty:   1_000,
			}},
			Asks: []kraken.BookLevel{{
				Price: *bookAsk,
				Qty:   1_000,
			}},
			Timestamp: at,
		}},
		CrossSection: types.NewCrossSection(),
		Advanced:     types.StreamAll,
	}
}

/*
TestSignal_CalculateUsesMarketSources proves ticker summary fields cannot
substitute for executed volume or reconstructed book spread.
*/
func TestSignal_CalculateUsesMarketSources(t *testing.T) {
	Convey("Given ticker summaries without authoritative market sources", t, func() {
		at := time.Date(2026, 7, 19, 11, 0, 0, 0, time.UTC)
		withoutTrades := sourceFrame(at, 100, 0, 1_000_000, 20, 2)
		withoutBook := sourceFrame(at, 100, 10, 1_000_000, 20, 2)
		withoutBook.Books = nil

		Convey("Then ticker fields are not used as hidden substitutes", func() {
			measurements, err := newSignal().Calculate(withoutTrades)
			So(err, ShouldBeNil)
			So(measurements, ShouldBeEmpty)

			measurements, err = newSignal().Calculate(withoutBook)
			So(err, ShouldBeNil)
			So(measurements, ShouldBeEmpty)
		})
	})

	Convey("Given signals calibrated with identical market history", t, func() {
		startedAt := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
		calibrate := func(signal *Signal) {
			for index := range 5 {
				measurements, err := signal.Calculate(sourceFrame(
					startedAt.Add(time.Duration(index)*time.Second),
					100+float64(index),
					10,
					1_000+float64(index*1_000),
					20,
					2,
				))
				So(err, ShouldBeNil)
				So(measurements, ShouldNotBeEmpty)
			}
		}

		sincere := newSignal()
		phantom := newSignal()
		normalBook := newSignal()
		compressedBook := newSignal()
		calibrate(sincere)
		calibrate(phantom)
		calibrate(normalBook)
		calibrate(compressedBook)
		observedAt := startedAt.Add(5 * time.Second)

		Convey("Then identical ticker volume cannot replace executed lift", func() {
			sincereOutput, err := sincere.Calculate(sourceFrame(
				observedAt, 110, 200, 1_000_000, 20, 2,
			))
			So(err, ShouldBeNil)
			phantomOutput, err := phantom.Calculate(sourceFrame(
				observedAt, 110, 0, 1_000_000, 20, 2,
			))
			So(err, ShouldBeNil)
			sincereMetrics := indexEpoch(sincereOutput)
			phantomMetrics := indexEpoch(phantomOutput)

			So(sincereMetrics[types.MetricRVOL].Raw,
				ShouldBeGreaterThan, phantomMetrics[types.MetricRVOL].Raw)
			So(sincereMetrics[types.MetricIgnition].Raw,
				ShouldBeGreaterThan, phantomMetrics[types.MetricIgnition].Raw)
		})

		Convey("Then identical ticker spread cannot replace book compression", func() {
			normalOutput, err := normalBook.Calculate(sourceFrame(
				observedAt, 105, 20, 1_000_000, 20, 2,
			))
			So(err, ShouldBeNil)
			compressedOutput, err := compressedBook.Calculate(sourceFrame(
				observedAt, 105, 20, 1_000_000, 20, 0.2,
			))
			So(err, ShouldBeNil)
			normalMetrics := indexEpoch(normalOutput)
			compressedMetrics := indexEpoch(compressedOutput)

			So(normalMetrics[types.MetricSpread].Raw, ShouldAlmostEqual, 2)
			So(compressedMetrics[types.MetricSpread].Raw, ShouldAlmostEqual, 0.2)
			So(compressedMetrics[types.MetricCompression].Raw,
				ShouldBeGreaterThan, normalMetrics[types.MetricCompression].Raw)
		})
	})
}

/*
TestSignal_Interest proves all authoritative PumpDump inputs must advance the
signal state even though measurements close on ticker observations.
*/
func TestSignal_Interest(t *testing.T) {
	Convey("Given a PumpDump signal", t, func() {
		Convey("Then it subscribes to trade, book, and ticker cuts", func() {
			So(newSignal().Interest(), ShouldEqual, types.StreamAll)
		})
	})
}

func TestSignal_MeasureSkipsIncompleteRow(t *testing.T) {
	Convey("Given a partial Kraken ticker row", t, func() {
		signal := newSignal()

		Convey("When measure runs", func() {
			result, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{
					{Symbol: "BTC/USD"},
				},
				CrossSection: types.NewCrossSection(),
			})

			Convey("Then it should wait without publishing metrics", func() {
				So(err, ShouldBeNil)
				So(result, ShouldBeEmpty)
			})
		})
	})
}

func TestSignal_MeasureEmitsWhileCalibrating(t *testing.T) {
	Convey("Given a complete ticker that has not yet formed ignition baselines", t, func() {
		signal := newSignal()
		at := time.Date(2026, 7, 17, 1, 3, 45, 0, time.UTC)
		row := kraken.TickerData{
			Symbol:    "BTC/USD",
			Bid:       krakendecimal.NewFromFloat64(999),
			Ask:       krakendecimal.NewFromFloat64(1001),
			Last:      krakendecimal.NewFromFloat64(1000),
			Volume:    10,
			Timestamp: at,
		}

		Convey("When two ticks arrive before baselines form", func() {
			increment := krakendecimal.NewFromFloat64(0.1)
			first, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{row},
				Trades: []kraken.TradeData{{
					Symbol:    row.Symbol,
					Price:     *row.Last,
					Qty:       10,
					Timestamp: at,
				}},
				Books: []kraken.BookData{{
					Symbol:         row.Symbol,
					Type:           "snapshot",
					PriceIncrement: increment,
					Bids: []kraken.BookLevel{{
						Price: *row.Bid,
						Qty:   10,
					}},
					Asks: []kraken.BookLevel{{
						Price: *row.Ask,
						Qty:   10,
					}},
					Timestamp: at,
				}},
				CrossSection: types.NewCrossSection(),
				Advanced:     types.StreamAll,
			})
			So(err, ShouldBeNil)
			So(first, ShouldNotBeEmpty)

			second, err := signal.Calculate(&types.MarketFrame{
				Tickers: []kraken.TickerData{{
					Symbol:    row.Symbol,
					Bid:       row.Bid,
					Ask:       row.Ask,
					Last:      row.Last,
					Volume:    row.Volume,
					Timestamp: at.Add(time.Second),
				}},
				CrossSection: types.NewCrossSection(),
				Advanced:     types.StreamTicker,
			})

			Convey("Then the second tick still publishes provisional ignition evidence", func() {
				So(err, ShouldBeNil)
				So(second, ShouldNotBeEmpty)

				found := false

				for _, measurement := range second {
					if measurement.Metric == types.MetricIgnition {
						found = true
						So(measurement.Validity.State, ShouldEqual, types.ValidityProvisional)
					}
				}

				So(found, ShouldBeTrue)
			})
		})
	})
}

func BenchmarkSignal_Measure(benchmark *testing.B) {
	market := conditions.PumpDump()

	benchmark.ReportAllocs()

	for benchmark.Loop() {
		_, err := drive(newSignal(), market.Frames())

		if err != nil {
			benchmark.Fatal(err)
		}
	}
}
