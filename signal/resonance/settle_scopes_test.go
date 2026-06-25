package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/qpool"
)

func TestSignalSettleScopes(testingTB *testing.T) {
	Convey("Given a batched resonance signal with multiple symbols", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.02, 8)

		defer func() {
			_ = signal.Close()
		}()

		So(signal.err, ShouldBeNil)
		So(signal.engine, ShouldBeNil)

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}

		for index, scope := range scopes {
			last := 100.0 + float64(index)*10

			insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})
			insertFeedArtifact(signal, "book", scope, []bookFixture{{
				Symbol: scope,
				Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
				Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should settle batch measurements from tree-hydrated sensory vectors", func() {
			results, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)
			So(signal.engine, ShouldNotBeNil)
			So(len(results), ShouldBeGreaterThan, 0)
		})
	})
}

func TestSignalSettleScopesGrowsWithUniverse(testingTB *testing.T) {
	Convey("Given a signal seeded with batch size 1 but a larger live universe", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.02, 1)

		defer func() {
			_ = signal.Close()
		}()

		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		scopes := []string{"UNFI/USD", "BDXN/USD", "SRM/USD", "SLX/USD", "TITCOIN/USD"}

		for index, scope := range scopes {
			last := 100.0 + float64(index)*10

			insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})
			insertFeedArtifact(signal, "book", scope, []bookFixture{{
				Symbol: scope,
				Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
				Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should grow capacity and settle every obscure mover, dropping none", func() {
			results, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)
			So(signal.batchSize, ShouldBeGreaterThanOrEqualTo, len(scopes))
			So(len(results), ShouldEqual, len(scopes))
		})
	})
}

func TestSignalSettleScopesSkipsUnchanged(testingTB *testing.T) {
	Convey("Given a settled resonance scope", testingTB, func() {
		viper.Set("signals.feed_ring_capacity", 64)

		ctx := context.Background()
		pool := qpool.NewQ[any](ctx, 2, 4, nil)
		signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.02, 8)

		defer func() {
			_ = signal.Close()
		}()

		scope := "BTC/USD"
		observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
			Symbol:    scope,
			Last:      100,
			Volume:    1000,
			ChangePct: 0.01,
			Timestamp: observedAt,
		}})
		insertFeedArtifact(signal, "book", scope, []bookFixture{{
			Symbol: scope,
			Bids:   []bookLevelFixture{{Price: 99, Qty: 1}},
			Asks:   []bookLevelFixture{{Price: 101, Qty: 1}},
		}})

		first, firstErr := signal.SettleScopes([]string{scope})

		So(firstErr, ShouldBeNil)
		So(len(first), ShouldEqual, 1)

		second, secondErr := signal.SettleScopes([]string{scope})

		So(secondErr, ShouldBeNil)

		Convey("It should skip settling unchanged scopes", func() {
			So(len(second), ShouldEqual, 0)
		})
	})
}

func BenchmarkSignalSettleScopes(b *testing.B) {
	viper.Set("signals.feed_ring_capacity", 64)

	ctx := context.Background()
	pool := qpool.NewQ[any](ctx, 2, 4, nil)
	signal := NewSignal(ctx, pool, dmt.NewTree(""), nil, 0.01, 128)

	defer signal.Close()

	observedAt := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	scopes := make([]string, 128)

	for index := range scopes {
		scope := fmt.Sprintf("SYM%d/USD", index)
		scopes[index] = scope
		last := 100.0 + float64(index)

		insertFeedArtifact(signal, "ticker", scope, []tickerFixture{{
			Symbol:    scope,
			Last:      last,
			Volume:    1000,
			ChangePct: 0.01,
			Timestamp: observedAt,
		}})
		insertFeedArtifact(signal, "book", scope, []bookFixture{{
			Symbol: scope,
			Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
			Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
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
	signal := NewSignal(ctx, nil, dmt.NewTree(""), nil, 0.01, 128)

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

	if ensureErr := signal.ensureCapacity(len(entries)); ensureErr != nil {
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
