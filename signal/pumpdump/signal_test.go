package pumpdump

import (
	"context"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/signal/testutil"
	bookfixtures "github.com/theapemachine/symm/tests/fixtures/book"
	tradefixtures "github.com/theapemachine/symm/tests/fixtures/trade"
)

type tickerCase struct {
	symbol    string
	stamp     int64
	volume    float64
	last      float64
	bid       float64
	ask       float64
	change    float64
	changePct float64
}

type priorCase struct {
	key         string
	symbol      string
	stamp       int64
	volume      float64
	last        float64
	volumeDelta float64
	logReturn   float64
	spread      float64
	bookSpread  float64
	tradeVolume float64
	touchDepth  float64
	rvol        float64
	compression float64
	decline     float64
}

func TestMeasureRequiresCrossSection(t *testing.T) {
	signal := NewSignal(context.Background(), dmt.NewTree(""))
	result := testutil.FirstMeasured(signal.Measure(tickerArtifact(tickerCase{
		symbol: "BTC/USD", stamp: 1, volume: 1000, last: 100, bid: 99, ask: 101,
	}), nil))

	if result != nil {
		t.Fatalf("Measure yielded with nil cross-section")
	}
}

func TestTreeMeasurementsRebuildBaselines(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "TREE/USD"

	insertPrior(t, signal, priorCase{
		key: "old", symbol: symbol, stamp: 100, volume: 1000, last: 100,
		volumeDelta: 100, logReturn: 0.001, spread: 2, bookSpread: 2,
		tradeVolume: 100, rvol: 1, compression: 0,
	})

	insertPrior(t, signal, priorCase{
		key: "new", symbol: symbol, stamp: 200, volume: 1120, last: 101,
		volumeDelta: 120, logReturn: 0.001, spread: 2, bookSpread: 2,
		tradeVolume: 120, rvol: 1, compression: 0,
	})

	seedPeers(t, crossSection, 300, 1, 1200)
	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: 300, volume: 1320, last: 103, bid: 102, ask: 104,
	})

	if result == nil {
		t.Fatal("Measure returned nil")
	}

	if got := datura.Peek[float64](result, "volumeDelta"); got != 200 {
		t.Fatalf("volumeDelta=%v, want 200 from tree prior", got)
	}

	if got := datura.Peek[float64](result, "output", "rvol"); got <= 1 {
		t.Fatalf("rvol=%v, want lift above prior median", got)
	}
}

func TestBookArtifactDrivesCompression(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "BOOK/USD"

	insertPrior(t, signal, priorCase{
		key: "baseline", symbol: symbol, stamp: 100, volume: 1000, last: 100,
		volumeDelta: 100, logReturn: 0.001, spread: 10, bookSpread: 10,
		tradeVolume: 100, rvol: 1, compression: 0,
	})
	insertBook(t, signal, symbol, 190, 99.5, 100.5, 500, 500)
	seedPeers(t, crossSection, 200, 1, 1000)

	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: 200, volume: 1100, last: 100, bid: 95, ask: 105,
	})

	if result == nil {
		t.Fatal("Measure returned nil")
	}

	if got := datura.Peek[float64](result, "bookSpread"); got != 1 {
		t.Fatalf("bookSpread=%v, want 1", got)
	}

	if got := datura.Peek[float64](result, "output", "compression"); got <= 0 {
		t.Fatalf("compression=%v, want book-derived tightening", got)
	}
}

func TestTradeVolumeDrivesLiftWithFlatTickerVolume(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "TRADE/USD"

	insertPrior(t, signal, priorCase{
		key: "baseline", symbol: symbol, stamp: int64(time.Second),
		volume: 1000, last: 100, volumeDelta: 20, logReturn: 0.001,
		spread: 2, bookSpread: 2, tradeVolume: 20, rvol: 1, compression: 0,
	})
	insertTrade(t, signal, symbol, int64(2*time.Second), "buy", 100, 200)
	seedPeers(t, crossSection, int64(3*time.Second), 1, 1000)

	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: int64(3 * time.Second),
		volume: 1000, last: 101, bid: 100, ask: 102,
	})

	if result == nil {
		t.Fatal("Measure returned nil")
	}

	if got := datura.Peek[float64](result, "tradeVolume"); got != 200 {
		t.Fatalf("tradeVolume=%v, want 200", got)
	}

	if got := datura.Peek[float64](result, "output", "rvol"); got <= 1 {
		t.Fatalf("rvol=%v, want trade-volume lift despite flat ticker volume", got)
	}
}

func TestMeasureKrakenFixtureStreamEnrichesTicker(testingTB *testing.T) {
	Convey("Given Kraken book and trade fixture streams in the tree", testingTB, func() {
		signal := newTestSignal(testingTB)
		crossSection := testutil.NewTestCrossSection(testingTB)
		symbol := "MATIC/USD"
		base := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)

		for artifact := range bookfixtures.NewFixture(bookfixtures.SNAPSHOT, 1).Artifacts() {
			artifact.WithScope(symbol)
			artifact.SetTimestamp(base.UnixNano())
			insertMarketTestArtifact(signal, artifact)
			artifact.Release()
			break
		}

		index := 0
		for artifact := range tradefixtures.NewFixture(tradefixtures.UPDATE, 3).Artifacts() {
			artifact.WithScope(symbol)
			artifact.SetTimestamp(base.Add(time.Duration(index+1) * time.Second).UnixNano())
			insertMarketTestArtifact(signal, artifact)
			artifact.Release()
			index++
		}

		seedPeers(testingTB, crossSection, base.Add(10*time.Second).UnixNano(), 1, 1000)

		Convey("When pumpdump measures the next ticker tick", func() {
			result := measureTicker(testingTB, signal, crossSection, tickerCase{
				symbol: symbol,
				stamp:  base.Add(10 * time.Second).UnixNano(),
				volume: 997038.98383185,
				last:   0.5667,
				bid:    0.5666,
				ask:    0.5668,
			})

			Convey("Then the measurement should carry book and trade evidence", func() {
				So(result, ShouldNotBeNil)
				So(datura.Peek[float64](result, "bookSpread"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](result, "touchDepth"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](result, "tradeVolume"), ShouldBeGreaterThan, 0)
				So(datura.Peek[float64](result, "output", "rvol"), ShouldBeGreaterThan, 0)
			})
		})
	})
}

func TestCoiledCompressionRequiresBookTightening(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "COIL/USD"

	insertPrior(t, signal, priorCase{
		key: "baseline", symbol: symbol, stamp: 100, volume: 1000, last: 100,
		volumeDelta: 100, logReturn: 0.001, spread: 10, bookSpread: 10,
		tradeVolume: 100, rvol: 1, compression: 0,
	})
	insertBook(t, signal, symbol, 190, 99.95, 100.05, 900, 900)
	seedPeers(t, crossSection, 200, 1, 1000)

	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: 200, volume: 1100, last: 100, bid: 95, ask: 105,
	})

	if result == nil {
		t.Fatal("Measure returned nil")
	}

	if categoryResult(result) != logic.CategoryIndex(logic.CategoryCoiledCompression) {
		t.Fatalf("category=%d, want coiled compression", categoryResult(result))
	}

	if outputScore(result, "compression") <= outputScore(result, "ignition") {
		t.Fatalf("compression=%v ignition=%v", outputScore(result, "compression"), outputScore(result, "ignition"))
	}
}

func TestOrganicTrendRequiresPeerRelativeModeration(t *testing.T) {
	signal := newTestSignal(t)
	crossSection := testutil.NewTestCrossSection(t)
	symbol := "TREND/USD"

	warmTrend(t, signal, crossSection, symbol)
	result := measureTicker(t, signal, crossSection, tickerCase{
		symbol: symbol, stamp: int64(9 * time.Second), volume: 1600, last: 108, bid: 107, ask: 109,
	})

	if result == nil {
		t.Fatal("Measure returned nil")
	}

	if categoryResult(result) != logic.CategoryIndex(logic.CategoryOrganicTrend) {
		t.Fatalf("category=%d, want organic trend", categoryResult(result))
	}

	spikeSignal := newTestSignal(t)
	spikeCrossSection := testutil.NewTestCrossSection(t)
	warmTrend(t, spikeSignal, spikeCrossSection, symbol)
	spike := measureTicker(t, spikeSignal, spikeCrossSection, tickerCase{
		symbol: symbol, stamp: int64(9 * time.Second), volume: 4000, last: 130, bid: 129, ask: 131,
	})

	if spike == nil {
		t.Fatal("spike Measure returned nil")
	}

	if categoryResult(spike) == logic.CategoryIndex(logic.CategoryOrganicTrend) {
		t.Fatalf("isolated spike was classified as organic trend")
	}
}

func TestHistorySortsOutOfOrderPriors(t *testing.T) {
	signal := newTestSignal(t)
	symbol := "SORT/USD"

	insertPrior(t, signal, priorCase{
		key: "a", symbol: symbol, stamp: 300, volume: 300, last: 103,
		volumeDelta: 30, logReturn: 0.003, spread: 3, bookSpread: 3,
		tradeVolume: 30, rvol: 1.2, compression: 0.1,
	})
	insertPrior(t, signal, priorCase{
		key: "b", symbol: symbol, stamp: 100, volume: 100, last: 101,
		volumeDelta: 10, logReturn: 0.001, spread: 1, bookSpread: 1,
		tradeVolume: 10, rvol: 1, compression: 0,
	})

	history := signal.history(symbol, 300)

	if len(history.stamps) != 2 {
		t.Fatalf("history length=%d, want 2", len(history.stamps))
	}

	if history.stamps[0] != 100 || history.stamps[1] != 300 {
		t.Fatalf("stamps=%v, want sorted ascending", history.stamps)
	}

	if history.prevVolume != 300 {
		t.Fatalf("prevVolume=%v, want latest timestamp volume 300", history.prevVolume)
	}
}

func BenchmarkSignalMeasure(b *testing.B) {
	for b.Loop() {
		signal := newBenchSignal(b)
		crossSection := testutil.NewTestCrossSection(b)
		warmTrend(b, signal, crossSection, "BENCH/USD")
		result := measureTicker(b, signal, crossSection, tickerCase{
			symbol: "BENCH/USD", stamp: int64(9 * time.Second), volume: 1600, last: 108, bid: 107, ask: 109,
		})

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		if outputScore(result, "confidence") <= 0 {
			b.Fatal("Measure returned zero confidence")
		}
	}
}
