package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

func TestSignalSettleScopes(testingTB *testing.T) {
	Convey("Given a batched resonance signal with multiple symbols", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, nil, 0.02, 8)

		defer func() {
			_ = signal.Close()
		}()

		So(signal.err, ShouldBeNil)
		So(signal.engine, ShouldBeNil)

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

		for index, scope := range scopes {
			last := 100.0 + float64(index)*10

			signal.ticker.Update(krakenmarket.TickerUpdates{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})

			signal.book.Update(krakenmarket.BookUpdates{{
				Symbol: scope,
				Bids:   []krakenmarket.BookLevel{{Price: last - 1, Qty: 1}},
				Asks:   []krakenmarket.BookLevel{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should settle all scopes in one batch call", func() {
			results, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)
			So(signal.engine, ShouldNotBeNil)
			So(len(results), ShouldEqual, len(scopes))

			for _, scope := range scopes {
				measurement, ok := results[scope]

				So(ok, ShouldBeTrue)
				So(string(measurement.Source), ShouldEqual, "resonance")
				So(measurement.Symbol, ShouldEqual, scope)
				So(measurement.Surprise, ShouldBeGreaterThanOrEqualTo, 0)
				So(measurement.Confidence, ShouldBeGreaterThan, 0)
				So(string(measurement.Category), ShouldBeIn,
					[]string{CategoryFlow, CategoryStress, CategoryCoupling},
				)
			}

			first := results[scopes[0]]

			So(string(first.Category), ShouldBeIn,
				[]string{CategoryFlow, CategoryStress, CategoryCoupling},
			)
		})
	})
}

func BenchmarkSignalSettleScopes(b *testing.B) {
	viper.Set("signals.feed_ring_capacity", 64)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, nil)
	signal := NewSignal(ctx, pool, nil, 0.01, 128)

	defer signal.Close()

	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scopes := make([]string, 128)

	for index := range scopes {
		scope := fmt.Sprintf("SYM%d/USD", index)
		scopes[index] = scope
		last := 100.0 + float64(index)

		signal.ticker.Update(krakenmarket.TickerUpdates{{
			Symbol:    scope,
			Last:      last,
			Volume:    1000,
			ChangePct: 0.01,
			Timestamp: observedAt,
		}})

		signal.book.Update(krakenmarket.BookUpdates{{
			Symbol: scope,
			Bids:   []krakenmarket.BookLevel{{Price: last - 1, Qty: 1}},
			Asks:   []krakenmarket.BookLevel{{Price: last + 1, Qty: 1}},
		}})
	}

	if _, warmErr := signal.SettleScopes(scopes[:1]); warmErr != nil {
		b.Fatal(warmErr)
	}

	if signal.engine == nil {
		b.Fatal(signal.err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, _ = signal.SettleScopes(scopes)
	}
}

func BenchmarkMetalBatchEngineSettle(b *testing.B) {
	ctx := context.Background()
	signal := NewSignal(ctx, nil, nil, 0.01, 128)

	defer signal.Close()

	entries := make([]batchEntry, 128)

	for index := range entries {
		scope := fmt.Sprintf("SYM%d/USD", index)

		slot, assigned := signal.slots.assign(scope)

		if !assigned {
			b.Fatalf("slot assignment failed for %s", scope)
		}

		entries[index] = batchEntry{
			slot:   slot,
			symbol: scope,
			input:  make([]float64, SensoryChannelCount),
		}

		for channel := range entries[index].input {
			entries[index].input[channel] = 0.1 + float64(channel)*0.01
		}
	}

	if ensureErr := signal.ensureEngine(); ensureErr != nil {
		b.Fatal(ensureErr)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		_, settleErr := signal.engine.Settle(entries)

		if settleErr != nil {
			b.Fatal(settleErr)
		}
	}
}
