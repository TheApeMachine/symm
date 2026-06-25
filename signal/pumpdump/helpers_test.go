package pumpdump

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/testutil"
)

func newTestSignal(t testing.TB) *Signal {
	t.Helper()

	signal := NewSignal(context.Background(), dmt.NewTree(""))

	if signal == nil {
		t.Fatal("NewSignal returned nil")
	}

	return signal
}

func newBenchSignal(b *testing.B) *Signal {
	b.Helper()

	return newTestSignal(b)
}

func warmTrend(t testing.TB, signal *Signal, crossSection *market.CrossSection, symbol string) {
	t.Helper()

	for tick := 0; tick < 8; tick++ {
		stamp := int64(tick+1) * int64(time.Second)
		seedPeers(t, crossSection, stamp, tick, 1800)
		result := measureTicker(t, signal, crossSection, tickerCase{
			symbol: symbol,
			stamp:  stamp,
			volume: 1000 + float64(tick)*100,
			last:   100 + float64(tick),
			bid:    99 + float64(tick),
			ask:    101 + float64(tick),
		})

		if result == nil {
			t.Fatalf("warm tick %d returned nil", tick)
		}
	}
}

func seedPeers(t testing.TB, crossSection *market.CrossSection, stamp int64, tick int, volume float64) {
	t.Helper()

	for peerIndex := 0; peerIndex < 3; peerIndex++ {
		price := 100 + float64(tick) + float64(peerIndex)
		row, err := market.NewSymbolRow(
			fmt.Sprintf("PEER-%d/USD", peerIndex),
			price,
			0.01,
			volume,
			0,
			time.Unix(0, stamp),
		)

		if err != nil {
			t.Fatal(err)
		}

		if err = crossSection.Observe(row); err != nil {
			t.Fatal(err)
		}
	}
}

func measureTicker(
	t testing.TB,
	signal *Signal,
	crossSection *market.CrossSection,
	input tickerCase,
) *datura.Artifact {
	t.Helper()

	result := testutil.FirstMeasured(signal.Measure(tickerArtifact(input), crossSection))
	signal.tree = testutil.StoreMeasurement(signal.tree, result)

	return result
}

func tickerArtifact(input tickerCase) *datura.Artifact {
	payload := fmt.Appendf(nil,
		`{"channel":"ticker","type":"update","data":[{"symbol":%q,"bid":%g,"bid_qty":500,"ask":%g,"ask_qty":500,"last":%g,"volume":%g,"change":%g,"change_pct":%g}]}`,
		input.symbol, input.bid, input.ask, input.last, input.volume, input.change, input.changePct,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("ticker")
	artifact.WithScope("update")
	artifact.WithPayload(payload)
	artifact.SetTimestamp(input.stamp)

	return artifact
}

func insertPrior(t testing.TB, signal *Signal, input priorCase) {
	t.Helper()

	measurement := datura.Acquire("pumpdump", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(input.symbol)
	_ = measurement.SetOrigin(string(logic.SourcePumpDump))
	measurement.SetTimestamp(input.stamp)
	measurement.Merge("volume", input.volume)
	measurement.Merge("last", input.last)
	measurement.Merge("volumeDelta", input.volumeDelta)
	measurement.Merge("logReturn", input.logReturn)
	measurement.Merge("spread", input.spread)
	measurement.Merge("bookSpread", input.bookSpread)
	measurement.Merge("tradeVolume", input.tradeVolume)
	measurement.Merge("timestamp", float64(input.stamp))
	measurement.MergeOutput("rvol", input.rvol)
	measurement.MergeOutput("compression", input.compression)
	measurement.MergeOutput("rvolDecline", input.decline)

	key := []byte(fmt.Sprintf("measurement/%s/%s/%s", input.symbol, logic.SourcePumpDump, input.key))
	signal.tree, _ = signal.tree.InsertArtifact(key, measurement)
}

func insertBook(
	t testing.TB,
	signal *Signal,
	symbol string,
	stamp int64,
	bid, ask, bidQuantity, askQuantity float64,
) {
	t.Helper()

	payload := fmt.Appendf(nil,
		`{"channel":"book","type":"update","data":[{"symbol":%q,"bids":[{"price":%g,"qty":%g}],"asks":[{"price":%g,"qty":%g}]}]}`,
		symbol, bid, bidQuantity, ask, askQuantity,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("book")
	artifact.WithScope("update")
	artifact.WithPayload(payload)
	artifact.SetTimestamp(stamp)
	signal.tree, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}

func insertTrade(t testing.TB, signal *Signal, symbol string, stamp int64, side string, price, quantity float64) {
	t.Helper()

	payload := fmt.Appendf(nil,
		`{"channel":"trade","type":"update","data":[{"symbol":%q,"side":%q,"price":%g,"qty":%g}]}`,
		symbol, side, price, quantity,
	)
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole("trade")
	artifact.WithScope("update")
	artifact.WithPayload(payload)
	artifact.SetTimestamp(stamp)
	signal.tree, _ = signal.tree.InsertArtifact(artifact.Prefix("role", "timestamp"), artifact)
}

func categoryResult(result *datura.Artifact) int {
	categories := []logic.CategoryType{
		logic.CategoryVerticalIgnition,
		logic.CategoryCoiledCompression,
		logic.CategoryOrganicTrend,
		logic.CategoryFadedExhaustion,
	}

	return testutil.DominantCategoryIndex(result, categories)
}

func outputScore(result *datura.Artifact, key string) float64 {
	score := datura.Peek[float64](result, "output", key)

	if math.IsNaN(score) || math.IsInf(score, 0) {
		return 0
	}

	return score
}
