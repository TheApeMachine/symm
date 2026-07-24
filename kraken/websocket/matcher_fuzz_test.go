package websocket

import (
	"context"
	"testing"
	"time"

	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
)

/*
testClock advances deterministic time for fuzz replay without wall sleeps.
*/
type testClock struct {
	now time.Time
}

func (clock *testClock) Now() time.Time {
	return clock.now
}

func (clock *testClock) Sleep(_ context.Context, wait time.Duration) error {
	clock.now = clock.now.Add(wait)

	return nil
}

/*
FuzzMatcherFundedFill proves fills never drive seeded balances negative.
*/
func FuzzMatcherFundedFill(f *testing.F) {
	f.Add("buy", 0.01)
	f.Add("sell", 0.005)

	f.Fuzz(func(t *testing.T, side string, quantity float64) {
		if side != "buy" && side != "sell" {
			t.Skip()
		}

		if quantity <= 0 || quantity > 0.2 {
			t.Skip()
		}

		clock := &testClock{now: time.Unix(1_700_000_000, 0).UTC()}
		matcher := NewMatcher(clock, "USD", 10_000, 0.0026)
		matcher.SetMark("BTC/USD", 50_000)
		matcher.SeedBalance("BTC", 0.2)

		_, err := matcher.Fill(side, "BTC/USD", quantity, 0)

		for asset, total := range matcher.balances {
			if total < 0 {
				t.Fatalf("negative balance for %s after fill err=%v", asset, err)
			}
		}
	})
}

/*
FuzzPaperReplayIdentity proves replay preserves execution identifiers.
*/
func FuzzPaperReplayIdentity(f *testing.F) {
	f.Add("PAPER-00026", "PAPER-00025", "BTCUSD", "buy", 0.0002, 64129.9)

	f.Fuzz(func(
		t *testing.T,
		execID, orderID, pair, side string,
		volume, price float64,
	) {
		if execID == "" || orderID == "" || pair == "" || volume <= 0 || price <= 0 {
			t.Skip()
		}

		paper := NewPaper(context.Background(), NewSimulator(), configFixture())
		sub := paper.Subscribe("executions")
		trades := []any{map[string]any{
			"id": execID, "order_id": orderID, "pair": pair, "side": side,
			"volume": volume, "price": price, "cost": volume * price,
			"fee": 0.01, "status": "filled", "time": "2026-07-14T21:02:56Z",
		}}

		if err := paper.Replay(trades); err != nil {
			t.Fatalf("replay: %v", err)
		}

		raw := (<-sub.Channel).([]byte)
		execution := kraken.NewExecution(raw)

		if len(execution.Data) != 1 {
			t.Fatalf("expected one execution, got %d", len(execution.Data))
		}

		if execution.Data[0].ExecID != execID {
			t.Fatalf("exec id mismatch: got %q want %q", execution.Data[0].ExecID, execID)
		}
	})
}

func configFixture() config.Config {
	return config.Config{
		System: config.SystemConfig{ActorBuffer: 64},
		Market: config.MarketConfig{QuoteCurrency: "USD"},
	}
}
