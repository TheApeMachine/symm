package trader

import (
	"testing"

	"github.com/theapemachine/symm/types"

	. "github.com/smartystreets/goconvey/convey"
)

func TestChunkerDrain(t *testing.T) {
	Convey("Given ticker and trade streams with events at different timestamps", t, func() {
		pool := testPool()
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		signal := &Signal{CrossSection: crossSection}
		ticker := NewTicker(pool, signal, testUIHub())
		trade := NewTrade(pool, signal, testUIHub())

		chunker := NewChunker(crossSection, map[string]types.Drainer{
			"ticker": ticker,
			"trade":  trade,
		}, []string{"ticker", "trade"})

		// The trade event's exchange timestamp is earlier than the ticker
		// event's, even though trade is registered after ticker in the
		// chunker's stream order, so a correct merge must still place the
		// trade event first.
		pushRing(trade.ring, []byte(`[{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-04T12:00:00Z"}]`))
		pushRing(ticker.ring, []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:01Z"}]`))

		Convey("When the chunker drains", func() {
			chunks, snapshot, drainErr := chunker.Drain()

			Convey("Then one symbol chunk merges both streams in event-time order", func() {
				So(drainErr, ShouldBeNil)
				So(chunks, ShouldHaveLength, 1)
				So(chunks[0].Symbol, ShouldEqual, "BTC/USD")
				So(chunks[0].Events, ShouldHaveLength, 2)
				So(chunks[0].Events[0].Stream, ShouldEqual, "trade")
				So(chunks[0].Events[1].Stream, ShouldEqual, "ticker")
				So(chunks[0].Events[0].At.Before(chunks[0].Events[1].At), ShouldBeTrue)
				So(chunks[0].Watermark, ShouldResemble, chunks[0].Events[1].At)
				So(snapshot.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}

func TestChunkerDrainGroupsBySymbol(t *testing.T) {
	Convey("Given trade events for two symbols in one drain cycle", t, func() {
		pool := testPool()
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		signal := &Signal{CrossSection: crossSection}
		trade := NewTrade(pool, signal, testUIHub())

		chunker := NewChunker(crossSection, map[string]types.Drainer{
			"trade": trade,
		}, []string{"trade"})

		pushRing(trade.ring, []byte(`[
			{"symbol":"ETH/USD","side":"buy","price":10,"qty":1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-04T12:00:00Z"},
			{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"limit","trade_id":2,"timestamp":"2026-07-04T12:00:01Z"}
		]`))

		Convey("When the chunker drains", func() {
			chunks, _, drainErr := chunker.Drain()

			Convey("Then it returns one chunk per symbol, ordered by symbol name", func() {
				So(drainErr, ShouldBeNil)
				So(chunks, ShouldHaveLength, 2)
				So(chunks[0].Symbol, ShouldEqual, "BTC/USD")
				So(chunks[1].Symbol, ShouldEqual, "ETH/USD")
				So(chunks[0].Events, ShouldHaveLength, 1)
				So(chunks[1].Events, ShouldHaveLength, 1)
			})
		})
	})
}

func TestChunkerMeasureSharesFrozenCrossSection(t *testing.T) {
	Convey("Given a trade stream and a ticker stream sharing one cross-section", t, func() {
		pool := testPool()
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		tickerSignal := &recordingSignal{}
		tradeSignal := &recordingSignal{}
		signal := &Signal{
			CrossSection: crossSection,
			Ticker:       []types.Signal[any]{tickerSignal},
			Trade:        []types.Signal[any]{tradeSignal},
		}
		ticker := NewTicker(pool, signal, testUIHub())
		trade := NewTrade(pool, signal, testUIHub())

		chunker := NewChunker(crossSection, map[string]types.Drainer{
			"ticker": ticker,
			"trade":  trade,
		}, []string{"ticker", "trade"})

		pushRing(ticker.ring, []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`))
		pushRing(trade.ring, []byte(`[{"symbol":"ETH/USD","side":"buy","price":10,"qty":1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-04T12:00:01Z"}]`))

		Convey("When the chunker drains and measures", func() {
			chunks, snapshot, drainErr := chunker.Drain()
			So(drainErr, ShouldBeNil)

			measurements, measureErr := chunker.Measure(chunks, snapshot)

			Convey("Then the trade signal sees the same populated cross-section the ticker observed, not a throwaway empty one", func() {
				So(measureErr, ShouldBeNil)
				So(measurements, ShouldHaveLength, 2)
				So(tradeSignal.crossSection, ShouldNotBeNil)
				So(tradeSignal.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
				So(tickerSignal.crossSection.Symbols(), ShouldResemble, []string{"BTC/USD"})
			})
		})
	})
}

func TestChunkerSnapshotIsFrozenPerWatermark(t *testing.T) {
	Convey("Given a chunker that drains ticker events across two cycles", t, func() {
		pool := testPool()
		crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
		So(err, ShouldBeNil)

		signal := &Signal{CrossSection: crossSection}
		ticker := NewTicker(pool, signal, testUIHub())
		chunker := NewChunker(crossSection, map[string]types.Drainer{
			"ticker": ticker,
		}, []string{"ticker"})

		pushRing(ticker.ring, []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`))
		_, firstSnapshot, firstErr := chunker.Drain()
		So(firstErr, ShouldBeNil)

		Convey("When a later cycle observes a new symbol", func() {
			pushRing(ticker.ring, []byte(`[{"symbol":"ETH/USD","bid":9,"ask":11,"last":10,"volume":5,"timestamp":"2026-07-04T12:00:01Z"}]`))
			_, secondSnapshot, secondErr := chunker.Drain()

			Convey("Then the earlier watermark's snapshot never observes it", func() {
				So(secondErr, ShouldBeNil)
				So(firstSnapshot.Symbols(), ShouldResemble, []string{"BTC/USD"})
				So(secondSnapshot.Symbols(), ShouldResemble, []string{"BTC/USD", "ETH/USD"})
			})
		})
	})
}

func TestChunkerMergeInvariantToFrameChunking(t *testing.T) {
	Convey("Given the same trade rows delivered as one frame or as several", t, func() {
		rows := []string{
			`{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-04T12:00:00Z"}`,
			`{"symbol":"ETH/USD","side":"buy","price":10,"qty":1,"ord_type":"limit","trade_id":2,"timestamp":"2026-07-04T12:00:01Z"}`,
			`{"symbol":"BTC/USD","side":"sell","price":101,"qty":2,"ord_type":"limit","trade_id":3,"timestamp":"2026-07-04T12:00:02Z"}`,
		}

		drainAsOneFrame := func() []types.Event {
			pool := testPool()
			crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
			So(err, ShouldBeNil)

			trade := NewTrade(pool, &Signal{CrossSection: crossSection}, testUIHub())
			pushRing(trade.ring, []byte("["+rows[0]+","+rows[1]+","+rows[2]+"]"))

			events, drainErr := trade.Drain()
			So(drainErr, ShouldBeNil)

			return events
		}

		drainAsSeparateFrames := func() []types.Event {
			pool := testPool()
			crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
			So(err, ShouldBeNil)

			trade := NewTrade(pool, &Signal{CrossSection: crossSection}, testUIHub())

			for _, row := range rows {
				pushRing(trade.ring, []byte("["+row+"]"))
			}

			events, drainErr := trade.Drain()
			So(drainErr, ShouldBeNil)

			return events
		}

		Convey("When drained as a single frame versus one frame per row", func() {
			oneFrame := drainAsOneFrame()
			manyFrames := drainAsSeparateFrames()

			Convey("Then the resulting event order is identical regardless of frame chunking", func() {
				So(len(oneFrame), ShouldEqual, len(manyFrames))

				for index := range oneFrame {
					So(oneFrame[index].Symbol, ShouldEqual, manyFrames[index].Symbol)
					So(oneFrame[index].At, ShouldResemble, manyFrames[index].At)
					So(oneFrame[index].Price, ShouldEqual, manyFrames[index].Price)
				}
			})
		})
	})
}

func BenchmarkChunkerDrain(b *testing.B) {
	pool := testPool()
	crossSection, err := types.NewCrossSection(types.DefaultCrossSectionConfig())
	if err != nil {
		b.Fatal(err)
	}

	signal := &Signal{CrossSection: crossSection}
	ticker := NewTicker(pool, signal, nil)
	trade := NewTrade(pool, signal, nil)
	chunker := NewChunker(crossSection, map[string]types.Drainer{
		"ticker": ticker,
		"trade":  trade,
	}, []string{"ticker", "trade"})

	tickerRaw := []byte(`[{"symbol":"BTC/USD","bid":99,"ask":101,"last":100,"volume":12.5,"timestamp":"2026-07-04T12:00:00Z"}]`)
	tradeRaw := []byte(`[{"symbol":"BTC/USD","side":"buy","price":100,"qty":1,"ord_type":"limit","trade_id":1,"timestamp":"2026-07-04T12:00:00Z"}]`)

	b.ReportAllocs()
	for b.Loop() {
		pushRing(ticker.ring, tickerRaw)
		pushRing(trade.ring, tradeRaw)

		if _, _, err := chunker.Drain(); err != nil {
			b.Fatal(err)
		}
	}
}
