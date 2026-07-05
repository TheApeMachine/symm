package resonance

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSignalSettleScopes(t *testing.T) {
	Convey("Given a batched resonance signal with multiple symbols", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scopes := []string{"BTC/USD", "ETH/USD", "SOL/USD"}
		seedUniverse(t, signal, scopes, startAt(0))

		Convey("When scopes are settled", func() {
			results, err := signal.SettleScopes(scopes)

			Convey("Then batch measurements should be returned", func() {
				So(err, ShouldBeNil)
				So(signal.engine, ShouldNotBeNil)
				So(results, ShouldHaveLength, len(scopes))
			})
		})
	})
}

func TestSignalSettleScopesGrowsWithUniverse(t *testing.T) {
	Convey("Given a signal seeded with batch size one", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 1)
		defer func() { _ = signal.Close() }()

		scopes := []string{"UNFI/USD", "BDXN/USD", "SRM/USD", "SLX/USD", "TITCOIN/USD"}
		seedUniverse(t, signal, scopes, startAt(0))

		Convey("When a larger live universe is settled", func() {
			results, err := signal.SettleScopes(scopes)

			Convey("Then capacity should grow without dropping symbols", func() {
				So(err, ShouldBeNil)
				So(signal.batchSize, ShouldBeGreaterThanOrEqualTo, len(scopes))
				So(results, ShouldHaveLength, len(scopes))
			})
		})
	})
}

func TestSignalSettleScopesSkipsUnchanged(t *testing.T) {
	Convey("Given a settled resonance scope", t, func() {
		signal := NewSignal(context.Background(), nil, 0.02, 8)
		defer func() { _ = signal.Close() }()

		scope := "BTC/USD"
		seedMarket(t, signal, scope, 100, 1000, 0.01, 0.01, startAt(0))

		first, firstErr := signal.SettleScopes([]string{scope})
		second, secondErr := signal.SettleScopes([]string{scope})

		Convey("When the scope is settled twice without new data", func() {
			Convey("Then the second settle should skip unchanged input", func() {
				So(firstErr, ShouldBeNil)
				So(first, ShouldHaveLength, 1)
				So(secondErr, ShouldBeNil)
				So(second, ShouldBeEmpty)
			})
		})
	})
}

func BenchmarkSignalSettleScopes(b *testing.B) {
	scopes := make([]string, 128)
	for index := range scopes {
		scopes[index] = fmt.Sprintf("SYM%d/USD", index)
	}

	b.ReportAllocs()

	for b.Loop() {
		signal := NewSignal(context.Background(), nil, 0.01, 128)
		seedUniverse(b, signal, scopes, startAt(0))

		if _, err := signal.SettleScopes(scopes); err != nil {
			b.Fatal(err)
		}

		_ = signal.Close()
	}
}

func BenchmarkMetalBatchEngineSettle(b *testing.B) {
	signal := NewSignal(context.Background(), nil, 0.01, 128)
	defer func() { _ = signal.Close() }()

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

	if err := signal.ensureCapacity(len(entries)); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for b.Loop() {
		if _, err := signal.engine.Settle(entries); err != nil {
			b.Fatal(err)
		}
	}
}

func seedUniverse(
	t testing.TB,
	signal *Signal,
	scopes []string,
	observedAt time.Time,
) {
	t.Helper()

	for index, scope := range scopes {
		last := 100.0 + float64(index)
		seedMarket(
			t,
			signal,
			scope,
			last,
			1000+float64(index),
			0.01*float64(index+1),
			0.01,
			observedAt.Add(time.Duration(index)*time.Second),
		)
	}
}
