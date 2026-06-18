package resonance

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/qpool"
)

func updateFeed(signal *Signal, role, scope string, payload any) {
	raw, err := json.Marshal(payload)

	if err != nil {
		panic(err)
	}

	artifact := datura.Acquire("kraken", datura.Artifact_Type_json)
	artifact.WithRole(role)
	artifact.WithScope(scope)
	artifact.WithPayload(raw)
	_ = signal.Update(artifact)
	artifact.Release()
}

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

			updateFeed(signal, "ticker", scope, []tickerFixture{{
				Symbol:    scope,
				Last:      last,
				Volume:    1000 + float64(index),
				ChangePct: 0.01 * float64(index+1),
				Timestamp: observedAt,
			}})
			updateFeed(signal, "book", scope, []bookFixture{{
				Symbol: scope,
				Bids:   []bookLevelFixture{{Price: last - 1, Qty: 1}},
				Asks:   []bookLevelFixture{{Price: last + 1, Qty: 1}},
			}})
		}

		Convey("It should withhold batch settlement until sensory vectors are available", func() {
			results, settleErr := signal.SettleScopes(scopes)

			So(settleErr, ShouldBeNil)
			So(signal.engine, ShouldNotBeNil)
			So(len(results), ShouldEqual, 0)
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

		updateFeed(signal, "ticker", scope, []tickerFixture{{
			Symbol:    scope,
			Last:      last,
			Volume:    1000,
			ChangePct: 0.01,
			Timestamp: observedAt,
		}})
		updateFeed(signal, "book", scope, []bookFixture{{
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
