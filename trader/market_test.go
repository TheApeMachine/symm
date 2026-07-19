package trader

import (
	"context"
	"testing"
	"time"

	krakendecimal "github.com/krakenfx/api-go/v2/pkg/decimal"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/symm/broker"
)

const marketBookFrame = `{
	"channel":"book",
	"type":"snapshot",
	"data":[{
		"symbol":"BTC/USD",
		"bids":[{"price":"1000","qty":2}],
		"asks":[{"price":"1000.1","qty":2}],
		"timestamp":"2026-07-17T01:03:45Z"
	}]
}`

const marketInstrumentFrame = `{
	"channel":"instrument",
	"type":"snapshot",
	"data":{"pairs":[{
		"symbol":"BTC/USD",
		"base":"BTC",
		"quote":"USD",
		"status":"offline",
		"tick_size":"0.1",
		"price_increment":"0.1"
	}]}
}`

const marketTickerFrame = `{
	"channel":"ticker",
	"type":"update",
	"data":[
		{
			"symbol":"BTC/USD",
			"bid":"999",
			"bid_qty":2,
			"ask":"1001",
			"ask_qty":2,
			"last":"1000",
			"volume":1,
			"vwap":1000,
			"change_pct":5,
			"timestamp":"2026-07-17T01:03:45Z"
		},
		{
			"symbol":"ETH/USD",
			"bid":"99",
			"bid_qty":5,
			"ask":"101",
			"ask_qty":5,
			"last":"100",
			"volume":100,
			"vwap":100,
			"change_pct":1,
			"timestamp":"2026-07-17T01:03:45Z"
		}
	]
}`

/*
TestNewMarket verifies that central feed configuration is rejected during
construction rather than after the public websocket has started delivering rows.
*/
func TestNewMarket(t *testing.T) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 4)
	viper.Set("signals.feed_track_capacity", 3)

	Convey("Given an invalid per-symbol feed capacity", t, func() {
		market, err := NewMarket(context.Background(), nil, nil)

		Convey("Then construction fails before handlers are registered", func() {
			So(market, ShouldBeNil)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid feed configuration")
		})
	})
}

/*
TestMarket_Cut verifies that the central market cut includes the peer state
required by every cross-sectional signal.
*/
func TestMarket_Cut(t *testing.T) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 4)
	viper.Set("signals.feed_track_capacity", 4)
	cutAt := time.Date(2026, 7, 17, 1, 3, 46, 0, time.UTC)

	Convey("Given current ticker rows retained by the central market", t, func() {
		market, err := NewMarket(context.Background(), nil, nil)
		So(err, ShouldBeNil)

		market.OnTicker([]byte(marketTickerFrame))

		Convey("When Market.Cut freezes the current journals", func() {
			frame, err := market.Cut(cutAt)
			So(err, ShouldBeNil)

			Convey("Then it carries the cross-section derived from that same cut", func() {
				So(frame.CrossSection, ShouldNotBeNil)
				So(frame.CrossSection.Metrics, ShouldHaveLength, 2)

				leader, threshold := frame.CrossSection.Leadership()
				So(leader, ShouldEqual, "BTC/USD")
				So(threshold, ShouldBeGreaterThan, 0)
			})

			Convey("Then a quiet later cut stays empty so the planner does not busy-spin", func() {
				later, err := market.Cut(cutAt.Add(time.Second))
				So(err, ShouldBeNil)
				So(later.IsEmpty(), ShouldBeTrue)
			})

			Convey("Then a book-only later cut carries only the active symbol's ticker", func() {
				instrument := broker.NewInstrument(nil, nil, nil)
				instrument.On([]byte(marketInstrumentFrame))
				bookMarket, err := NewMarket(context.Background(), nil, instrument)
				So(err, ShouldBeNil)
				bookMarket.OnTicker([]byte(marketTickerFrame))
				_, err = bookMarket.Cut(cutAt)
				So(err, ShouldBeNil)
				bookMarket.OnBook([]byte(marketBookFrame))
				later, err := bookMarket.Cut(cutAt.Add(time.Second))
				So(err, ShouldBeNil)
				So(later.Tickers, ShouldHaveLength, 1)
				So(later.Tickers[0].Symbol, ShouldEqual, "BTC/USD")
				So(later.Books, ShouldHaveLength, 1)
			})
		})

		Convey("When ingress exceeds the configured retained window", func() {
			market.OnTicker([]byte(marketTickerFrame))
			market.OnTicker([]byte(marketTickerFrame))
			bounded, err := market.Cut(cutAt)

			Convey("Then only the configured newest rows survive", func() {
				So(err, ShouldBeNil)
				So(bounded.Tickers, ShouldHaveLength, 2)
				later, err := market.Cut(cutAt.Add(time.Second))
				So(err, ShouldBeNil)
				So(later.IsEmpty(), ShouldBeTrue)
			})
		})
	})
}

/*
TestMarket_OnBook verifies that the central book owner enriches exchange rows
with the instrument increment required for tick-normalized calculations.
*/
func TestMarket_OnBook(t *testing.T) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	t.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	t.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 4)
	viper.Set("signals.feed_track_capacity", 4)
	cutAt := time.Date(2026, 7, 17, 1, 3, 46, 0, time.UTC)

	Convey("Given current Kraken instrument metadata", t, func() {
		instrument := broker.NewInstrument(nil, nil, nil)
		instrument.On([]byte(marketInstrumentFrame))
		market, err := NewMarket(context.Background(), nil, instrument)
		So(err, ShouldBeNil)

		Convey("When a raw public book snapshot arrives", func() {
			market.OnBook([]byte(marketBookFrame))
			frame, err := market.Cut(cutAt)
			So(err, ShouldBeNil)

			Convey("Then the retained row carries the exchange increment", func() {
				So(frame.Books, ShouldHaveLength, 1)
				So(
					frame.Books[0].PriceIncrement.Cmp(
						krakendecimal.NewFromFloat64(0.1),
					),
					ShouldEqual,
					0,
				)
			})
		})
	})
}

/*
TestMarket_WaitDirty verifies that WaitDirty returns one coalesced wake per
ingress burst instead of firing on the first message, and that it extends the
merge window while ingress keeps arriving up to the budget deadline.
*/
func TestMarket_WaitDirty(t *testing.T) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	previousWindow := viper.Get("signals.coalesce_window")
	t.Cleanup(func() {
		viper.Set("signals.feed_timeline_capacity", previousTimeline)
		viper.Set("signals.feed_track_capacity", previousTrack)
		viper.Set("signals.coalesce_window", previousWindow)
	})
	viper.Set("signals.feed_timeline_capacity", 4)
	viper.Set("signals.feed_track_capacity", 4)
	viper.Set("signals.coalesce_window", 10*time.Millisecond)

	Convey("Given a single buffered ingress signal", t, func() {
		market, err := NewMarket(context.Background(), nil, nil)
		So(err, ShouldBeNil)

		market.dirtyWake()
		start := time.Now()
		market.WaitDirty(200 * time.Millisecond)
		elapsed := time.Since(start)

		Convey("It coalesces one wake well under the budget and drains the channel", func() {
			So(elapsed, ShouldBeLessThan, 150*time.Millisecond)

			select {
			case <-market.dirty:
				So("dirty channel should be drained", ShouldBeEmpty)
			default:
			}
		})
	})

	Convey("Given ingress that keeps arriving inside the merge window", t, func() {
		market, err := NewMarket(context.Background(), nil, nil)
		So(err, ShouldBeNil)

		stop := make(chan struct{})

		go func() {
			ticker := time.NewTicker(3 * time.Millisecond)
			defer ticker.Stop()

			for {
				select {
				case <-stop:
					return
				case <-ticker.C:
					market.dirtyWake()
				}
			}
		}()

		market.dirtyWake()
		start := time.Now()

		go func() {
			time.Sleep(40 * time.Millisecond)
			close(stop)
		}()

		market.WaitDirty(300 * time.Millisecond)
		elapsed := time.Since(start)

		Convey("It extends past a single window yet returns before the budget", func() {
			So(elapsed, ShouldBeGreaterThan, 25*time.Millisecond)
			So(elapsed, ShouldBeLessThan, 300*time.Millisecond)
		})
	})
}

/*
BenchmarkMarket_Cut measures the complete central cut, including the shared
cross-sectional projection consumed by concurrent signals.
*/
func BenchmarkMarket_Cut(b *testing.B) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	b.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	b.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	market, err := NewMarket(context.Background(), nil, nil)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	cutAt := time.Date(2026, 7, 17, 1, 3, 46, 0, time.UTC)

	for b.Loop() {
		market.OnTicker([]byte(marketTickerFrame))

		if _, err := market.Cut(cutAt); err != nil {
			b.Fatal(err)
		}
	}
}

/*
BenchmarkMarket_OnBook measures central decoding and instrument enrichment for
the public book path that supplies all book-driven signals.
*/
func BenchmarkMarket_OnBook(b *testing.B) {
	previousTimeline := viper.Get("signals.feed_timeline_capacity")
	previousTrack := viper.Get("signals.feed_track_capacity")
	b.Cleanup(func() { viper.Set("signals.feed_timeline_capacity", previousTimeline) })
	b.Cleanup(func() { viper.Set("signals.feed_track_capacity", previousTrack) })
	viper.Set("signals.feed_timeline_capacity", 128)
	viper.Set("signals.feed_track_capacity", 128)
	instrument := broker.NewInstrument(nil, nil, nil)
	instrument.On([]byte(marketInstrumentFrame))
	market, err := NewMarket(context.Background(), nil, instrument)

	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		market.OnBook([]byte(marketBookFrame))
	}
}
