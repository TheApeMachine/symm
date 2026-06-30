package optimizer

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

func TestOptimizeUsesMCTSRewardToSelectPlaybookMutation(t *testing.T) {
	baseTree := []byte(`
branches:
  - condition_group:
      boolean: and
      conditions:
        - type: is_true
          left:
            type: holding
            holding:
              held: false
        - type: is_true
          left:
            source: sentiment
            type: category
            category:
              type: risk_on_surge
    action:
      type: market
      side: buy
`)
	frames := []ReplayFrame{{
		Time: time.Unix(1, 0).UTC(),
		Artifacts: []ReplayArtifact{{
			Role:      "ticker",
			Timestamp: time.Unix(1, 0).UnixNano(),
			Payload: map[string]any{
				"channel": "ticker",
				"data": []map[string]any{{
					"symbol": "ETH/USD",
					"last":   100.0,
				}},
			},
		}},
		Prices: map[string]float64{"ETH/USD": 100},
	}}

	report, tree, err := optimizeWithEvaluator(baseTree, frames, Options{
		Iterations: 80,
		MaxDepth:   1,
	}, fakeEvaluator{})
	if err != nil {
		t.Fatalf("optimize: %v", err)
	}

	if report.Best.Reward <= report.Baseline.Reward {
		t.Fatalf("best reward = %v, baseline = %v", report.Best.Reward, report.Baseline.Reward)
	}
	if len(report.Best.Mutations) == 0 || report.Best.Mutations[0] != "disable_entry_sentiment" {
		t.Fatalf("best mutations = %#v", report.Best.Mutations)
	}
	if bytes.Contains(tree, []byte("source: sentiment")) {
		t.Fatalf("optimized tree still contains disabled source:\n%s", tree)
	}
}

func TestReplayJSONLRoundTrip(t *testing.T) {
	frames := []ReplayFrame{{
		Time: time.Unix(10, 0).UTC(),
		Artifacts: []ReplayArtifact{{
			Role:      "ticker",
			Scope:     "BTC/USD",
			Timestamp: time.Unix(10, 0).UnixNano(),
			Payload: map[string]any{
				"channel": "ticker",
				"data": []map[string]any{{
					"symbol": "BTC/USD",
					"last":   65000.0,
				}},
			},
		}},
		Prices: map[string]float64{"BTC/USD": 65000},
	}}

	var buffer bytes.Buffer
	if err := WriteReplayJSONL(&buffer, frames); err != nil {
		t.Fatalf("write replay: %v", err)
	}

	decoded, err := ReadReplayJSONL(strings.NewReader(buffer.String()))
	if err != nil {
		t.Fatalf("read replay: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("frames = %d, want 1", len(decoded))
	}
	if decoded[0].Prices["BTC/USD"] != 65000 {
		t.Fatalf("price = %v, want 65000", decoded[0].Prices["BTC/USD"])
	}
}

func TestReplayEvaluatorScoresPlaybookOverFullFrameHorizon(t *testing.T) {
	treeYAML := []byte(`
branches:
  - condition_group:
      boolean: and
      conditions:
        - type: is_true
          left:
            type: holding
            holding:
              held: false
    action:
      type: limit
      side: buy
      entry_confidence: 1
`)
	frames := []ReplayFrame{
		replayTickerFrame("BTC/USD", 100, time.Unix(1, 0).UTC()),
		replayTickerFrame("BTC/USD", 120, time.Unix(2, 0).UTC()),
	}

	result, err := NewReplayEvaluator(frames, Options{
		InitialCash:  100,
		FeeRate:      0.004,
		MakerFeeRate: 0.0025,
		MaxPositions: 1,
	}).Evaluate(treeYAML)
	if err != nil {
		t.Fatalf("evaluate replay: %v", err)
	}
	if result.Trades == 0 {
		t.Fatalf("replay produced no trades")
	}
	if result.Wallet <= 100 {
		t.Fatalf("wallet = %v, want replay P/L above starting cash", result.Wallet)
	}
}

func TestReplayLedgerChargesMakerFeeForLimitEntries(t *testing.T) {
	options := Options{
		InitialCash:  100,
		FeeRate:      0.01,
		MakerFeeRate: 0.001,
		MaxPositions: 1,
	}
	prices := map[string]float64{"BTC/USD": 100}

	limitLedger := newLedger(options)
	limitLedger.Apply([]*datura.Artifact{
		replayTestAction("BTC/USD", logic.SideBuy, logic.ActionLimit, 1),
	}, prices)

	marketLedger := newLedger(options)
	marketLedger.Apply([]*datura.Artifact{
		replayTestAction("BTC/USD", logic.SideBuy, logic.ActionMarket, 1),
	}, prices)

	if limitLedger.wallet() <= marketLedger.wallet() {
		t.Fatalf("limit wallet = %v, market wallet = %v", limitLedger.wallet(), marketLedger.wallet())
	}
}

func TestReplayLedgerAppliesExitsBeforeEntries(t *testing.T) {
	ledger := newLedger(Options{
		InitialCash:  100,
		FeeRate:      0.01,
		MakerFeeRate: 0.001,
		MaxPositions: 1,
	})
	prices := map[string]float64{
		"BTC/USD": 100,
		"ETH/USD": 50,
	}

	ledger.Apply([]*datura.Artifact{
		replayTestAction("BTC/USD", logic.SideBuy, logic.ActionMarket, 1),
	}, prices)
	ledger.Apply([]*datura.Artifact{
		replayTestAction("ETH/USD", logic.SideBuy, logic.ActionMarket, 1),
		replayTestAction("BTC/USD", logic.SideSell, logic.ActionSettlePosition, 0),
	}, prices)

	if _, ok := ledger.positions["BTC/USD"]; ok {
		t.Fatalf("BTC position still open after same-frame exit")
	}
	if _, ok := ledger.positions["ETH/USD"]; !ok {
		t.Fatalf("ETH entry did not open after same-frame exit freed the slot")
	}
}

func replayTestAction(symbol string, side logic.Side, actionType logic.ActionType, confidence float64) *datura.Artifact {
	return datura.Acquire("test", datura.APPJSON).
		WithRole(string(side)).
		WithScope(symbol).
		WithPayload(datura.Map[any]{
			"symbol":           symbol,
			"side":             string(side),
			"type":             string(actionType),
			"entry_confidence": confidence,
		}.Marshal())
}

func replayTickerFrame(symbol string, price float64, stamp time.Time) ReplayFrame {
	return ReplayFrame{
		Time: stamp,
		Artifacts: []ReplayArtifact{{
			Origin:    "kraken:public",
			Role:      "ticker",
			Scope:     symbol,
			Type:      "update",
			Timestamp: stamp.UnixNano(),
			Payload: map[string]any{
				"channel": "ticker",
				"type":    "update",
				"data": []map[string]any{{
					"symbol":     symbol,
					"last":       price,
					"bid":        price,
					"ask":        price,
					"volume":     1000.0,
					"change":     0.0,
					"change_pct": 0.0,
				}},
			},
		}},
		Prices: map[string]float64{symbol: price},
	}
}

type fakeEvaluator struct{}

func (fakeEvaluator) Evaluate(treeYAML []byte) (ReplayResult, error) {
	if bytes.Contains(treeYAML, []byte("source: sentiment")) {
		return ReplayResult{Reward: -0.1, Wallet: 180, Cash: 180, Trades: 1}, nil
	}

	return ReplayResult{Reward: 0.2, Wallet: 240, Cash: 240, Trades: 1}, nil
}
