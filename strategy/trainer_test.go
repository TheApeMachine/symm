package strategy

import (
	"context"
	"math"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/kraken"
)

func TestPriceRoundTripReadsInstrumentLayer(t *testing.T) {
	t.Parallel()

	price := broker.NewPrice(context.Background(), nil, nil, nil, nil)
	price.SetReferenceCash(mustDecimal(t, "200"))
	price.SetDefaultFee(kraken.TradeVolumeFee{Fee: decimal.NewFromFloat64(0.8)})

	tick := data.NewValue("tick", "frame", "current")
	raw, err := sonic.Marshal(kraken.InstrumentPair{
		Symbol:         "ES/USD",
		Base:           "ES",
		Quote:          "USD",
		Status:         "online",
		QtyPrecision:   8,
		PricePrecision: 4,
		CostPrecision:  5,
	})
	if err != nil {
		t.Fatalf("marshal instrument: %v", err)
	}
	tick.AppendJSON(raw, data.SymbolIndex(data.LayerInstrument, "ES/USD"))

	trainer := &Trainer{price: price}
	pnl, cost, err := trainer.priceRoundTrip(
		"ES/USD",
		[]*data.Value{tick},
		mustDecimal(t, "1.25"),
		mustDecimal(t, "1.40"),
	)
	if err != nil {
		t.Fatalf("RoundTrip ES/USD: %v", err)
	}
	if pnl == nil || cost == nil || cost.Sign() <= 0 {
		t.Fatalf("expected priced round trip, pnl=%v cost=%v", pnl, cost)
	}
}

func TestUpClearsFrictionPublishesEnterAndExit(t *testing.T) {
	t.Parallel()

	trainer := &Trainer{}
	got, err := trainer.publishExcursion(1, "ES/USD", "R01/R05", "R02/R10", true, true, 0.02)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got != "enter policy/current/R01/R05/enter.bin, exit policy/current/R02/R10/exit.bin" {
		t.Fatalf("published %s", got)
	}
}

func TestDownPublishesNoopOnBothFragments(t *testing.T) {
	t.Parallel()

	trainer := &Trainer{}
	got, err := trainer.publishExcursion(1, "ES/USD", "R01", "R04", false, false, -0.01)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got != "noop policy/current/R01/noop.bin, noop policy/current/R04/noop.bin" {
		t.Fatalf("published %s", got)
	}
}

func TestExitRequiresEnter(t *testing.T) {
	t.Parallel()

	trainer := &Trainer{}
	got, err := trainer.publishExcursion(1, "ES/USD", "R01/R05", "R02/R10", false, true, 0.18)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got != "noop policy/current/R01/R05/noop.bin, noop policy/current/R02/R10/noop.bin" {
		t.Fatalf("published %s", got)
	}
}

func TestUpWithoutFrictionPublishesNoopOnBothFragments(t *testing.T) {
	t.Parallel()

	trainer := &Trainer{}
	got, err := trainer.publishExcursion(1, "ES/USD", "R01/R05", "R02/R10", false, false, 0.001)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if got != "noop policy/current/R01/R05/noop.bin, noop policy/current/R02/R10/noop.bin" {
		t.Fatalf("published %s", got)
	}
}

func TestReplayTapeSplitsPrecursorAndHolding(t *testing.T) {
	t.Parallel()

	const (
		aTick = 12
		bTick = 20
		cTick = 28
	)

	ticks := make([]*data.Value, 0, cTick)
	for tick := uint64(1); tick <= cTick; tick++ {
		value := math.Exp(float64(tick) / 4)
		source, label := "hawkes", "arrival_rate:buy"
		if tick > bTick {
			source, label = "liquidity", "relative_spread"
		}
		ticks = append(ticks, signalTick(tick, source, label, value))
	}

	trainer := NewTrainer(context.Background(), nil, nil)
	precursor, holding, precursorTicks, holdingTicks, err := trainer.replayTape(
		"test", ticks, aTick, bTick, cTick,
	)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if precursorTicks != int(bTick-aTick+1) {
		t.Fatalf("precursor ticks = %d", precursorTicks)
	}
	if holdingTicks != int(cTick-bTick+1) {
		t.Fatalf("holding ticks = %d", holdingTicks)
	}
	if precursor != "" || holding != "" {
		t.Fatalf("path without a completed rule: precursor %q holding %q", precursor, holding)
	}
}

func signalTick(tick uint64, source, label string, value float64) *data.Value {
	frame := data.NewValue(data.TickOrigin, data.TickRole, data.TickScope)
	frame.Tick = tick
	frame.Timestamp = tick * 1_000_000_000
	data.AppendSignalVector(frame, source, "ES/USD", []float64{value}, label)
	return frame
}

func TestPolicyObjectKeyIsTheGrammarPath(t *testing.T) {
	t.Parallel()

	got := policyObjectKey("R01/R05/R02", "enter")
	if got != "policy/current/R01/R05/R02/enter.bin" {
		t.Fatalf("policy key = %s", got)
	}
}

func mustDecimal(t *testing.T, raw string) *decimal.Decimal {
	t.Helper()

	value, err := decimal.NewFromString(raw)
	if err != nil {
		t.Fatalf("decimal %s: %v", raw, err)
	}

	return value
}
