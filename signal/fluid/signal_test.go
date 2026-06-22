package fluid

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

// GOFLAGS=-ldflags=-checklinkname=0 is required for qpool linkname hooks (see Makefile).

func categoryResult(result *datura.Artifact) int {
	return int(datura.Peek[float64](result, "output", "category"))
}

var classifierInputs = []string{"laminarScore", "turbulentScore", "inertialScore", "viscousScore"}

func outputScore(result *datura.Artifact, key string) float64 {
	return datura.Peek[float64](result, "output", key)
}

func winningClassifierInput(result *datura.Artifact) string {
	bestKey := classifierInputs[0]
	bestScore := outputScore(result, bestKey)

	for _, key := range classifierInputs[1:] {
		score := outputScore(result, key)

		if score > bestScore {
			bestScore = score
			bestKey = key
		}
	}

	return bestKey
}

func newTestPool(t testing.TB) *qpool.Q[any] {
	if t != nil {
		t.Helper()
	}

	pool := qpool.NewQ[any](context.Background(), 2, 4, nil)

	if pool == nil && t != nil {
		t.Fatal("qpool.NewQ returned nil")
	}

	return pool
}

func configureFluidViper() {
	viper.Set("market.book_depth_levels", 10)
	viper.Set("signals.volume_clock_bars_per_day", 288)
	viper.Set("signals.fluid.tick_size", 0.01)
	viper.Set("signals.fluid.grid_half_width", 10)
	viper.Set("signals.fluid.integration_interval", 100*time.Millisecond)
	symbolConfigValue.Store(nil)
}

func marketDatapoint(channel, payload string, timestamp int64) *datura.Artifact {
	artifact := datura.Acquire("kraken:public", datura.APPJSON)
	artifact.WithRole(channel)
	artifact.WithScope("update")
	artifact.WithPayload([]byte(payload))
	artifact.SetTimestamp(timestamp)

	return artifact
}

func bookPayload(symbol string, bidPrice, bidQty, askPrice, askQty float64, feedType string) string {
	return fmt.Sprintf(
		`{"symbol":%q,"type":%q,"bids":[{"price":%g,"qty":%g},{"price":%g,"qty":%g}],"asks":[{"price":%g,"qty":%g},{"price":%g,"qty":%g}]}`,
		symbol, feedType,
		bidPrice, bidQty, bidPrice-0.01, bidQty,
		askPrice, askQty, askPrice+0.01, askQty,
	)
}

func measureBestLaminarFrame(
	signal *Signal,
	frames []struct {
		channel string
		payload string
	},
	base time.Time,
) *datura.Artifact {
	var (
		result         *datura.Artifact
		bestLaminar    float64
		bestConfidence float64
	)

	for index, frame := range frames {
		at := base.Add(time.Duration(index) * 100 * time.Millisecond).UnixNano()
		measured := measureMarketFrame(signal, frame.channel, frame.payload, at)

		if measured == nil {
			continue
		}

		laminarScore := outputScore(measured, "laminarScore")

		if laminarScore <= 0 {
			measured.Release()

			continue
		}

		confidence := datura.Peek[float64](measured, "output", "confidence")

		if laminarScore > bestLaminar ||
			(laminarScore == bestLaminar && confidence > bestConfidence) {
			if result != nil {
				result.Release()
			}

			result = measured
			bestLaminar = laminarScore
			bestConfidence = confidence

			continue
		}

		measured.Release()
	}

	return result
}

func tickerPayload(symbol string, last, bid, ask, volume, changePct float64) string {
	return fmt.Sprintf(
		`{"symbol":%q,"last":%g,"bid":%g,"bid_qty":5.0,"ask":%g,"ask_qty":5.0,"volume":%g,"change_pct":%g}`,
		symbol, last, bid, ask, volume, changePct,
	)
}

func tradePayload(symbol, side string, price, quantity float64) string {
	return fmt.Sprintf(
		`{"symbol":%q,"side":%q,"price":%g,"qty":%g}`,
		symbol, side, price, quantity,
	)
}

func measureMarketFrame(signal *Signal, channel, payload string, timestamp int64) *datura.Artifact {
	datapoint := marketDatapoint(channel, payload, timestamp)

	defer datapoint.Release()

	return signal.Measure(datapoint)
}

func laminarStabilityFrames(symbol string) []struct {
	channel string
	payload string
} {
	return []struct {
		channel string
		payload string
	}{
		{"ticker", tickerPayload(symbol, 100, 99.99, 100.01, 1000, 0.01)},
		{"book", bookPayload(symbol, 99.99, 5, 100.01, 5, "snapshot")},
		{"book", bookPayload(symbol, 99.99, 8, 100.01, 8, "update")},
		{"book", bookPayload(symbol, 100.01, 8, 100.03, 8, "update")},
	}
}

func TestSignalMeasureCategorySemantics(t *testing.T) {
	configureFluidViper()

	Convey("Given a warmed tight-spread stable book through Measure", t, func() {
		signal := NewSignal(context.Background(), newTestPool(t), dmt.NewTree(""))
		So(signal, ShouldNotBeNil)

		defer func() {
			_ = signal.Close()
		}()

		base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
		symbol := "ETH/EUR"
		signal.SetInstrumentTickSize(symbol, 0.01)
		frames := laminarStabilityFrames(symbol)
		result := measureBestLaminarFrame(signal, frames, base)

		Convey("It should classify laminar stability with laminarScore winning", func() {
			So(result, ShouldNotBeNil)
			So(outputScore(result, "laminarScore"), ShouldBeGreaterThan, 0)
			So(outputScore(result, "laminarScore"), ShouldBeGreaterThan, outputScore(result, "turbulentScore"))
			So(outputScore(result, "laminarScore"), ShouldBeGreaterThan, outputScore(result, "inertialScore"))
			So(outputScore(result, "laminarScore"), ShouldBeGreaterThan, outputScore(result, "viscousScore"))
			So(winningClassifierInput(result), ShouldEqual, "laminarScore")
			So(categoryResult(result), ShouldEqual, 1)
			So(datura.Peek[float64](result, "output", "confidence"), ShouldBeGreaterThan, 0.25)

			result.Release()
		})
	})
}

func BenchmarkSignalMeasure(b *testing.B) {
	configureFluidViper()
	base := time.Date(2026, 5, 30, 12, 0, 0, 0, time.UTC)
	symbol := "ETH/EUR"

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), newTestPool(b), dmt.NewTree(""))

		if signal == nil {
			b.Fatal("NewSignal returned nil")
		}

		signal.SetInstrumentTickSize(symbol, 0.01)

		var (
			result         *datura.Artifact
			bestConfidence float64
		)

		frames := laminarStabilityFrames(symbol)

		for index, frame := range frames {
			at := base.Add(time.Duration(index) * 250 * time.Millisecond).UnixNano()
			measured := measureMarketFrame(signal, frame.channel, frame.payload, at)

			if measured == nil {
				continue
			}

			confidence := datura.Peek[float64](measured, "output", "confidence")

			if confidence > bestConfidence {
				if result != nil {
					result.Release()
				}

				result = measured
				bestConfidence = confidence

				continue
			}

			measured.Release()
		}

		if result == nil {
			b.Fatal("Measure returned nil")
		}

		result.Release()
		_ = signal.Close()
	}
}
